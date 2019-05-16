// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_cron_test", m, &testDB)
}

func TestETL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// This test relies on asynchronous things happening synchronously: namely
	// that queue processing happens before we start testing our assertions.
	// Setting GOMAXPROCS=1 enables this, but could concievably prove to be
	// fragile in the future.
	defer func(maxprocs int) {
		runtime.GOMAXPROCS(maxprocs)
	}(runtime.GOMAXPROCS(1))

	var (
		start    = postgres.NowTruncated()
		fooIndex = &internal.IndexVersion{
			Path:      "foo.com/foo",
			Timestamp: start,
			Version:   "v1.0.0",
		}
		barIndex = &internal.IndexVersion{
			Path:      "foo.com/bar",
			Timestamp: start.Add(time.Second),
			Version:   "v0.0.1",
		}
		fooProxy = proxy.NewTestVersion(t, fooIndex.Path, fooIndex.Version, map[string]string{
			"foo.go": "package foo\nconst Foo = \"Foo\"",
		})
		barProxy = proxy.NewTestVersion(t, barIndex.Path, barIndex.Version, map[string]string{
			"bar.go": "package bar\nconst Bar = \"Bar\"",
		})
		state = func(version *internal.IndexVersion, code, tryCount int) *internal.VersionState {
			status := &code
			if code == 0 {
				status = nil
			}
			return &internal.VersionState{
				ModulePath:     version.Path,
				IndexTimestamp: version.Timestamp,
				Status:         status,
				TryCount:       tryCount,
				Version:        version.Version,
			}
		}
		fooState = func(code, tryCount int) *internal.VersionState {
			return state(fooIndex, code, tryCount)
		}
		barState = func(code, tryCount int) *internal.VersionState {
			return state(barIndex, code, tryCount)
		}
	)

	tests := []struct {
		label    string
		index    []*internal.IndexVersion
		proxy    []*proxy.TestVersion
		requests []*http.Request
		wantFoo  *internal.VersionState
		wantBar  *internal.VersionState
	}{
		{
			label: "full fetch",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.TestVersion{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll-and-queue/", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusOK, 1),
		}, {
			label: "partial fetch",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.TestVersion{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll-and-queue/?limit=1", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
		}, {
			label: "fetch with errors",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.TestVersion{fooProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll-and-queue/", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusNotFound, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			teardownIndex, indexClient := index.SetupTestIndex(t, test.index)
			defer teardownIndex(t)

			teardownProxy, proxyClient := proxy.SetupTestProxy(t, test.proxy)
			defer teardownProxy(t)

			defer postgres.ResetTestDB(testDB, t)

			// Use 10 workers to have parallelism consistent with the cron binary.
			queue := NewInMemoryQueue(ctx, proxyClient, testDB, 10)

			s := NewServer(testDB, indexClient, proxyClient, queue, nil)

			for _, r := range test.requests {
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				if got, want := w.Code, http.StatusOK; got != want {
					t.Fatalf("Code = %d, want %d", got, want)
				}
			}

			// Gosched forces the queue to start processing requests to run, which
			// should not yield until all current requests have been scheduled. Note
			// that if GOMAXPROCS were > 1 here, this trick would not work.
			runtime.Gosched()
			queue.waitForTesting(ctx)

			// To avoid being a change detector, only look at ModulePath, Version,
			// Timestamp, and Status.
			ignore := cmpopts.IgnoreFields(internal.VersionState{},
				"CreatedAt", "NextProcessedAfter", "LastProcessedAt", "Error")

			got, err := testDB.GetVersionState(ctx, fooIndex.Path, fooIndex.Version)
			if err == nil {
				if diff := cmp.Diff(test.wantFoo, got, ignore); diff != "" {
					t.Errorf("testDB.GetVersionState(ctx, %q, %q) mismatch (-want +got):\n%s",
						fooIndex.Path, fooIndex.Version, diff)
				}
			} else if test.wantFoo == nil {
				if !derrors.IsNotFound(err) {
					t.Errorf("expected Not Found error for foo, got %v", err)
				}
			} else {
				t.Fatal(err)
			}
			got, err = testDB.GetVersionState(ctx, barIndex.Path, barIndex.Version)
			if err == nil {
				if diff := cmp.Diff(test.wantBar, got, ignore); diff != "" {
					t.Errorf("testDB.GetVersionState(ctx, %q, %q) mismatch (-want +got):\n%s",
						barIndex.Path, barIndex.Version, diff)
				}
			} else if test.wantBar == nil {
				if !derrors.IsNotFound(err) {
					t.Errorf("expected Not Found error for bar, got %v", err)
				}
			} else {
				t.Fatal(err)
			}
		})
	}
}
