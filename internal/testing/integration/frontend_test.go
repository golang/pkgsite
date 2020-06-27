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

	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var (
	in      = htmlcheck.In
	hasText = htmlcheck.HasText
)

func TestModulePackageDirectoryResolution(t *testing.T) {
	// The shared test state sets up the following scenario to exercise
	// what happens when a directory becomes a package,
	// and then becomes a directory again. Specifically:
	//  + at v1.2.3, github.com/golang/found/dir is a directory (containing dir/pkg)
	//  + at v1.2.4, github.com/golang/found/dir is a package
	//  + at v1.2.5, github.com/golang/found/dir is again just a directory
	versions := []*proxy.TestModule{
		{
			ModulePath: "github.com/golang/found",
			Version:    "v1.2.3",
			Files: map[string]string{
				"go.mod":         "module github.com/golang/found",
				"found.go":       "package found\nconst Value = 123",
				"dir/pkg/pkg.go": "package pkg\nconst Value = 321",
				"LICENSE":        testhelper.MITLicense,
			},
		},
		{
			ModulePath: "github.com/golang/found",
			Version:    "v1.2.4",
			Files: map[string]string{
				"go.mod":         "module github.com/golang/found",
				"found.go":       "package found\nconst Value = 124",
				"dir/pkg/pkg.go": "package pkg\nconst Value = 421",
				"dir/dir.go":     "package dir\nconst Value = \"I'm a package!\"",
				"LICENSE":        testhelper.MITLicense,
			},
		},
		{
			ModulePath: "github.com/golang/found",
			Version:    "v1.2.5",
			Files: map[string]string{
				"go.mod":         "module github.com/golang/found",
				"found.go":       "package found\nconst Value = 125",
				"dir/pkg/pkg.go": "package pkg\nconst Value = 521",
				"LICENSE":        testhelper.MITLicense,
			},
		},
	}

	tests := []struct {
		// Test description.
		desc string
		// URL Path (relative to the test server) to check
		urlPath string
		// The expected HTTP status code.
		wantCode int
		// If non-nil, used to verify the resulting page.
		want htmlcheck.Checker
	}{
		{
			desc:     "missing module",
			urlPath:  "/mod/github.com/golang/missing",
			wantCode: http.StatusNotFound,
		},
		{
			desc:     "latest module",
			urlPath:  "/mod/github.com/golang/found",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.5")),
				in(".DetailsHeader-badge", in(".DetailsHeader-badge--latest"))),
		},
		{
			desc:     "versioned module",
			urlPath:  "/mod/github.com/golang/found@v1.2.3",
			wantCode: http.StatusOK,
			want:     in(".DetailsHeader", hasText("v1.2.3")),
		},
		{
			desc:     "non-existent version",
			urlPath:  "/mod/github.com/golang/found@v1.1.3",
			wantCode: http.StatusNotFound,
			want: in(".Content",
				hasText("not available"),
				hasText("other versions of this module")),
		},
		{
			desc:     "latest package",
			urlPath:  "/github.com/golang/found/dir/pkg",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.5")),
				in(".DetailsContent", hasText("521")),
				in(".DetailsHeader-badge", in(".DetailsHeader-badge--latest"))),
		},
		{
			desc:     "earlier package",
			urlPath:  "/github.com/golang/found@v1.2.3/dir/pkg",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.3")),
				in(".DetailsContent", hasText("321"))),
		},
		{
			desc:     "dir is initially a directory",
			urlPath:  "/github.com/golang/found@v1.2.3/dir",
			wantCode: http.StatusOK,
			want:     in(".Directories", hasText("pkg")),
		},
		{
			desc:     "dir becomes a package",
			urlPath:  "/github.com/golang/found@v1.2.4/dir",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.4")),
				in(".DetailsContent", hasText("I'm a package"))),
		},
		{
			desc:     "dir becomes a directory again",
			urlPath:  "/github.com/golang/found@v1.2.5/dir",
			wantCode: http.StatusOK,
			want: in("",
				in(".Directories", hasText("pkg")),
				in(".DetailsHeader-badge", in(".DetailsHeader-badge--unknown"))),
		},
		{
			desc:     "latest package for /dir",
			urlPath:  "/github.com/golang/found/dir",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.4")),
				in(".DetailsContent", hasText("I'm a package"))),
		},
	}
	s, err := frontend.NewServer(frontend.ServerConfig{
		DataSource:           testDB,
		TaskIDChangeInterval: 10 * time.Minute,
		StaticPath:           "../../../content/static",
		ThirdPartyPath:       "../../../third_party",
		AppVersionLabel:      "",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)
	handler := middleware.LatestVersion(s.LatestVersion)(mux)
	ts := httptest.NewServer(handler)
	processVersions(context.Background(), t, versions)
	defer postgres.ResetTestDB(testDB, t)

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			resp, err := http.Get(ts.URL + test.urlPath)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != test.wantCode {
				t.Errorf("GET %s returned status %d, want %d", test.urlPath, resp.StatusCode, test.wantCode)
			}
			if test.want != nil {
				if err := htmlcheck.Run(resp.Body, test.want); err != nil {
					t.Error(err)
				}
			}
		})
	}

}

func processVersions(ctx context.Context, t *testing.T, testModules []*proxy.TestModule) {
	t.Helper()
	proxyClient, teardown := proxy.SetupTestProxy(t, testModules)
	defer teardown()
	sourceClient := source.NewClient(1 * time.Second)

	for _, tm := range testModules {
		res := fetch.FetchModule(ctx, tm.ModulePath, tm.Version, proxyClient, sourceClient)
		if res.Error != nil {
			t.Fatal(res.Error)
		}
		if err := testDB.InsertModule(ctx, res.Module); err != nil {
			t.Fatal(err)
		}
	}
}
