// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/static"
	thirdparty "golang.org/x/pkgsite/third_party"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

type testModule struct {
	path            string
	redistributable bool
	versions        []string
	packages        []testPackage
}

type testPackage struct {
	name           string
	suffix         string
	readmeContents string
	readmeFilePath string
	docs           []*internal.Documentation
}

func newTestServer(t *testing.T, proxyModules []*proxytest.Module, redisClient *redis.Client, experimentNames ...string) (*Server, http.Handler, func()) {
	t.Helper()
	proxyClient, teardown := proxytest.SetupTestClient(t, proxyModules)
	sourceClient := source.NewClient(sourceTimeout)
	ctx := context.Background()

	q := queue.NewInMemory(ctx, 1, experimentNames,
		func(ctx context.Context, mpath, version string) (int, error) {
			return FetchAndUpdateState(ctx, mpath, version, proxyClient, sourceClient, testDB)
		})

	s, err := NewServer(ServerConfig{
		DataSourceGetter:     func(context.Context) internal.DataSource { return testDB },
		Queue:                q,
		TaskIDChangeInterval: 10 * time.Minute,
		TemplateFS:           template.TrustedFSFromEmbed(static.FS),
		// Use the embedded FSs here to make sure they're tested.
		// Integration tests will use the actual directories.
		StaticFS:     static.FS,
		ThirdPartyFS: thirdparty.FS,
		StaticPath:   "../../static",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, redisClient, nil)

	var exps []*internal.Experiment
	for _, n := range experimentNames {
		exps = append(exps, &internal.Experiment{Name: n, Rollout: 100})
	}
	exp, err := middleware.NewExperimenter(ctx, time.Hour, func(context.Context) ([]*internal.Experiment, error) { return exps, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	mw := middleware.Experiment(exp)
	return s, mw(mux), func() {
		teardown()
		postgres.ResetTestDB(testDB, t)
	}
}

func insertTestModules(ctx context.Context, t *testing.T, mods []testModule) {
	for _, mod := range mods {
		var (
			suffixes []string
			pkgs     = make(map[string]testPackage)
		)
		for _, pkg := range mod.packages {
			suffixes = append(suffixes, pkg.suffix)
			pkgs[pkg.suffix] = pkg
		}
		for _, ver := range mod.versions {
			m := sample.Module(mod.path, ver, suffixes...)
			m.SourceInfo = source.NewGitHubInfo(sample.RepositoryURL, "", ver)
			m.IsRedistributable = mod.redistributable
			if !m.IsRedistributable {
				m.Licenses = nil
			}
			for _, u := range m.Units {
				if pkg, ok := pkgs[internal.Suffix(u.Path, m.ModulePath)]; ok {
					if pkg.name != "" {
						u.Name = pkg.name
					}
					if pkg.readmeContents != "" {
						u.Readme = &internal.Readme{
							Contents: pkg.readmeContents,
							Filepath: pkg.readmeFilePath,
						}
					}
					if pkg.docs != nil {
						u.Documentation = pkg.docs
					}
				}
				if !mod.redistributable {
					u.IsRedistributable = false
					u.Licenses = nil
					u.Documentation = nil
					u.Readme = nil
				}
			}
			postgres.MustInsertModule(ctx, t, testDB, m)
		}
	}
}
