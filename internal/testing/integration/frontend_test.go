// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

// TODO(https://github.com/golang/go/issues/40096): factor out this code reduce
// duplication
func setupFrontend(ctx context.Context, t *testing.T, q queue.Queue) *httptest.Server {
	t.Helper()
	s, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter:     func(context.Context) internal.DataSource { return testDB },
		TaskIDChangeInterval: 10 * time.Minute,
		StaticPath:           template.TrustedSourceFromConstant("../../../content/static"),
		ThirdPartyPath:       "../../../third_party",
		AppVersionLabel:      "",
		Queue:                q,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil, nil)

	// Get experiments from the context. Fully roll them out.
	expNames := experiment.FromContext(ctx).Active()
	var exps []*internal.Experiment
	for _, n := range expNames {
		exps = append(exps, &internal.Experiment{
			Name:    n,
			Rollout: 100,
		})
	}
	getter := func(context.Context) ([]*internal.Experiment, error) {
		return exps, nil
	}

	experimenter, err := middleware.NewExperimenter(ctx, 1*time.Minute, getter, nil)
	if err != nil {
		t.Fatal(err)
	}
	enableCSP := true
	mw := middleware.Chain(
		middleware.AcceptRequests(http.MethodGet, http.MethodPost),
		middleware.SecureHeaders(enableCSP),
		middleware.LatestVersions(s.GetLatestInfo),
		middleware.Experiment(experimenter),
	)
	return httptest.NewServer(mw(mux))
}

// TODO(https://github.com/golang/go/issues/40098): factor out this code reduce
// duplication
func setupQueue(ctx context.Context, t *testing.T, proxyModules []*proxy.Module, experimentNames ...string) (queue.Queue, func()) {
	cctx, cancel := context.WithCancel(ctx)
	proxyClient, teardown := proxy.SetupTestClient(t, proxyModules)
	sourceClient := source.NewClient(1 * time.Second)
	q := queue.NewInMemory(cctx, 1, experimentNames,
		func(ctx context.Context, mpath, version string) (_ int, err error) {
			return frontend.FetchAndUpdateState(ctx, mpath, version, proxyClient, sourceClient, testDB)
		})
	return q, func() {
		cancel()
		teardown()
	}
}

func processVersions(ctx context.Context, t *testing.T, testModules []*proxy.Module) {
	t.Helper()
	proxyClient, teardown := proxy.SetupTestClient(t, testModules)
	defer teardown()

	for _, tm := range testModules {
		fetchAndInsertModule(ctx, t, tm, proxyClient)
	}
}

func fetchAndInsertModule(ctx context.Context, t *testing.T, tm *proxy.Module, proxyClient *proxy.Client) {
	sourceClient := source.NewClient(1 * time.Second)
	res := fetch.FetchModule(ctx, tm.ModulePath, tm.Version, proxyClient, sourceClient, false)
	defer res.Defer()
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	postgres.MustInsertModule(ctx, t, testDB, res.Module)
}

func validateResponse(t *testing.T, method, testURL string, wantCode int, wantHTML htmlcheck.Checker) {
	t.Helper()

	var (
		resp *http.Response
		err  error
	)
	if method == http.MethodPost {
		resp, err = http.Post(testURL, "text/plain", nil)
	} else {
		resp, err = http.Get(testURL)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantCode {
		t.Fatalf("%q request to %q returned status %d, want %d", method, testURL, resp.StatusCode, wantCode)
	}

	if wantHTML != nil {
		if err := htmlcheck.Run(resp.Body, wantHTML); err != nil {
			t.Fatal(err)
		}
	}
}
