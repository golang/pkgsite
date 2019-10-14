// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/xerrors"
)

const testTimeout = 30 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_etl_test", m, &testDB)
}

func TestETL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	appVersionLabel = "20190000t000000"
	var (
		start    = sample.NowTruncated()
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
				AppVersion:     appVersionLabel,
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
				httptest.NewRequest("POST", "/poll-and-queue", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusOK, 1),
		}, {
			label: "partial fetch",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.TestVersion{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll-and-queue?limit=1", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
		}, {
			label: "fetch with errors",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.TestVersion{fooProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll-and-queue", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusNotFound, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			teardownIndex, indexClient := index.SetupTestIndex(t, test.index)
			defer teardownIndex(t)

			proxyClient, teardownProxy := proxy.SetupTestProxy(t, test.proxy)
			defer teardownProxy()

			defer postgres.ResetTestDB(testDB, t)

			// Use 10 workers to have parallelism consistent with the etl binary.
			queue := NewInMemoryQueue(ctx, proxyClient, testDB, 10)

			s, err := NewServer(testDB, indexClient, proxyClient, queue, "")
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			s.Install(mux.Handle)

			for _, r := range test.requests {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if got, want := w.Code, http.StatusOK; got != want {
					t.Fatalf("Code = %d, want %d", got, want)
				}
			}

			// Sleep to hopefully allow the work to begin processing, at which point
			// waitForTesting will successfully block until it is complete.
			// Experimentally this was not flaky with even 10ms sleep, but we bump to
			// 50ms to be extra careful.
			time.Sleep(50 * time.Millisecond)
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
				if !xerrors.Is(err, derrors.NotFound) {
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
				if !xerrors.Is(err, derrors.NotFound) {
					t.Errorf("expected Not Found error for bar, got %v", err)
				}
			} else {
				t.Fatal(err)
			}
		})
	}
}

func TestParseIntParam(t *testing.T) {
	for _, test := range []struct {
		in   string
		want int
	}{
		{"", -1},
		{"-1", -1},
		{"312", 312},
		{"bad", -1},
	} {
		got := parseIntParam(httptest.NewRequest("GET", fmt.Sprintf("/foo?x=%s", test.in), nil), "x", -1)
		if got != test.want {
			t.Errorf("%q: got %d, want %d", test.in, got, test.want)
		}
	}
}
