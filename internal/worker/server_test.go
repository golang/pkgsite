// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

const testTimeout = 60 * time.Second

var (
	testDB     *postgres.DB
	httpClient *http.Client
)

func TestMain(m *testing.M) {
	httpClient = &http.Client{Transport: fakeTransport{}}
	dochtml.LoadTemplates(template.TrustedSourceFromConstant("../../content/static/html/doc"))
	postgres.RunDBTests("discovery_worker_test", m, &testDB)
}

type debugExporter struct {
	t *testing.T
}

func (e debugExporter) ExportSpan(s *trace.SpanData) {
	e.t.Logf("⚡ %s: %v", s.Name, s.EndTime.Sub(s.StartTime))
}

func setupTraceDebugging(t *testing.T) {
	trace.RegisterExporter(debugExporter{t})
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
}

func TestWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	setupTraceDebugging(t)

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
		fooProxy = &proxy.Module{
			ModulePath: fooIndex.Path,
			Version:    fooIndex.Version,
			Files: map[string]string{
				"go.mod": "module foo.com/foo",
				"foo.go": "package foo\nconst Foo = \"Foo\"",
			},
		}
		barProxy = &proxy.Module{
			ModulePath: barIndex.Path,
			Version:    barIndex.Version,
			Files: map[string]string{
				"go.mod": "module foo.com/bar",
				"bar.go": "package bar\nconst Bar = \"Bar\"",
			},
		}
		state = func(version *internal.IndexVersion, code, tryCount, numPackages int) *internal.ModuleVersionState {
			goModPath := version.Path
			if code == 0 || code >= 300 {
				goModPath = ""
			}
			var n *int
			if code != 0 && code != http.StatusNotFound {
				n = &numPackages
			}
			return &internal.ModuleVersionState{
				ModulePath:     version.Path,
				IndexTimestamp: version.Timestamp,
				Status:         code,
				TryCount:       tryCount,
				Version:        version.Version,
				GoModPath:      goModPath,
				NumPackages:    n,
			}
		}
		fooState = func(code, tryCount int) *internal.ModuleVersionState {
			return state(fooIndex, code, tryCount, 1)
		}
		barState = func(code, tryCount int) *internal.ModuleVersionState {
			return state(barIndex, code, tryCount, 1)
		}
	)

	tests := []struct {
		label    string
		index    []*internal.IndexVersion
		proxy    []*proxy.Module
		requests []*http.Request
		wantFoo  *internal.ModuleVersionState
		wantBar  *internal.ModuleVersionState
	}{
		{
			label: "poll only",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.Module{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll", nil),
			},
			wantFoo: fooState(0, 0),
			wantBar: barState(0, 0),
		},
		{
			label: "full fetch",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.Module{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll", nil),
				httptest.NewRequest("POST", "/enqueue", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusOK, 1),
		}, {
			label: "partial fetch",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.Module{fooProxy, barProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll?limit=1", nil),
				httptest.NewRequest("POST", "/enqueue", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
		}, {
			label: "fetch with errors",
			index: []*internal.IndexVersion{fooIndex, barIndex},
			proxy: []*proxy.Module{fooProxy},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/poll", nil),
				httptest.NewRequest("POST", "/enqueue", nil),
			},
			wantFoo: fooState(http.StatusOK, 1),
			wantBar: barState(http.StatusNotFound, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			indexClient, teardownIndex := index.SetupTestIndex(t, test.index)
			defer teardownIndex()

			proxyClient, teardownProxy := proxy.SetupTestClient(t, test.proxy)
			defer teardownProxy()
			defer postgres.ResetTestDB(testDB, t)
			f := &Fetcher{proxyClient, source.NewClient(sourceTimeout), testDB}

			// Use 10 workers to have parallelism consistent with the worker binary.
			q := queue.NewInMemory(ctx, 10, nil, func(ctx context.Context, mpath, version string) (int, error) {
				code, _, err := f.FetchAndUpdateState(ctx, mpath, version, "", false)
				return code, err
			})

			s, err := NewServer(&config.Config{}, ServerConfig{
				DB:           testDB,
				IndexClient:  indexClient,
				ProxyClient:  proxyClient,
				SourceClient: f.SourceClient,
				Queue:        q,
			})
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
			// 100ms to be extra careful.
			time.Sleep(100 * time.Millisecond)
			q.WaitForTesting(ctx)

			// To avoid being a change detector, only look at ModulePath, Version,
			// Timestamp, and Status.
			ignore := cmpopts.IgnoreFields(internal.ModuleVersionState{},
				"CreatedAt", "NextProcessedAfter", "LastProcessedAt", "Error")

			got, err := testDB.GetModuleVersionState(ctx, fooIndex.Path, fooIndex.Version)
			if err == nil {
				if diff := cmp.Diff(test.wantFoo, got, ignore); diff != "" {
					t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q) mismatch (-want +got):\n%s",
						fooIndex.Path, fooIndex.Version, diff)
				}
			} else if test.wantFoo == nil {
				if !errors.Is(err, derrors.NotFound) {
					t.Errorf("expected Not Found error for foo, got %v", err)
				}
			} else {
				t.Fatal(err)
			}
			got, err = testDB.GetModuleVersionState(ctx, barIndex.Path, barIndex.Version)
			if err == nil {
				if diff := cmp.Diff(test.wantBar, got, ignore); diff != "" {
					t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q) mismatch (-want +got):\n%s",
						barIndex.Path, barIndex.Version, diff)
				}
			} else if test.wantBar == nil {
				if !errors.Is(err, derrors.NotFound) {
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
		got := parseLimitParam(httptest.NewRequest("GET", fmt.Sprintf("/foo?limit=%s", test.in), nil), -1)
		if got != test.want {
			t.Errorf("%q: got %d, want %d", test.in, got, test.want)
		}
	}
}

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		module  string
		version string
		err     error
	}{
		{
			name:    "ValidFetchURL",
			url:     "https://proxy.com/module/@v/v1.0.0",
			module:  "module",
			version: "v1.0.0",
			err:     nil,
		},
		{
			name: "InvalidFetchURL",
			url:  "https://proxy.com/",
			err:  errors.New(`invalid path: "/"`),
		},
		{
			name: "InvalidFetchURLNoModule",
			url:  "https://proxy.com/@v/version",
			err:  errors.New(`invalid path: "/@v/version"`),
		},
		{
			name: "InvalidFetchURLNoVersion",
			url:  "https://proxy.com/module/@v/",
			err:  errors.New(`invalid path: "/module/@v/"`),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Errorf("url.Parse(%q): %v", test.url, err)
			}

			m, v, err := parseModulePathAndVersion(u.Path)
			if test.err != nil {
				if err == nil {
					t.Fatalf("parseModulePathAndVersion(%q): error = nil; want = (%v)", u.Path, test.err)
				}
				if test.err.Error() != err.Error() {
					t.Fatalf("error = (%v); want = (%v)", err, test.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("error = (%v); want = (%v)", err, test.err)
			}

			if test.module != m || test.version != v {
				t.Fatalf("parseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
					u, m, v, err, test.module, test.version, test.err)
			}
		})
	}
}

func TestShouldDisableProxyFetch(t *testing.T) {
	for _, test := range []struct {
		status int
		want   bool
	}{
		{200, false},
		{490, false},
		{500, false},
		{520, true},
		{542, true},
		{580, false},
	} {

		got := shouldDisableProxyFetch(&internal.ModuleVersionState{
			ModulePath: "m",
			Version:    "v1.2.3",
			Status:     test.status,
		})
		if got != test.want {
			t.Errorf("status %d: got %t, want %t", test.status, got, test.want)
		}

	}
}

type fakeTransport struct{}

func (fakeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("bad")
}
