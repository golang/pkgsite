// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

// TODO(https://github.com/golang/go/issues/40096): factor out this code reduce
// duplication
func setupFrontend(ctx context.Context, t *testing.T, q queue.Queue, rc *redis.Client) *httptest.Server {
	t.Helper()
	const staticDir = "../../../static"
	fs := &frontend.FetchServer{
		Queue:                q,
		TaskIDChangeInterval: 10 * time.Minute,
	}
	s, err := frontend.NewServer(frontend.ServerConfig{
		FetchServer:      fs,
		DataSourceGetter: func(context.Context) internal.DataSource { return testDB },
		TemplateFS:       template.TrustedFSFromTrustedSource(template.TrustedSourceFromConstant(staticDir)),
		StaticFS:         os.DirFS(staticDir),
		ThirdPartyFS:     os.DirFS("../../../third_party"),
		Queue:            q,
		Config:           &config.Config{ServeStats: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	var cacher frontend.Cacher
	if rc != nil {
		cacher = middleware.NewCacher(rc)
	}
	s.Install(mux.Handle, cacher, nil)

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
		middleware.Experiment(experimenter),
	)
	return httptest.NewServer(mw(mux))
}

// TODO(https://github.com/golang/go/issues/40098): factor out this code reduce
// duplication
func setupQueue(ctx context.Context, t *testing.T, proxyModules []*proxytest.Module, experimentNames ...string) (queue.Queue, func()) {
	cctx, cancel := context.WithCancel(ctx)
	proxyClient, teardown := proxytest.SetupTestClient(t, proxyModules)
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

func processVersions(ctx context.Context, t *testing.T, testModules []*proxytest.Module) {
	t.Helper()
	proxyClient, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	for _, tm := range testModules {
		fetchAndInsertModule(ctx, t, tm, proxyClient)
	}
}

func fetchAndInsertModule(ctx context.Context, t *testing.T, tm *proxytest.Module, proxyClient *proxy.Client) {
	sourceClient := source.NewClient(1 * time.Second)
	res := fetch.FetchModule(ctx, tm.ModulePath, tm.Version, fetch.NewProxyModuleGetter(proxyClient, sourceClient))
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
