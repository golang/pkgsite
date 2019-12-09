// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/testing/htmlcheck"
	"golang.org/x/discovery/internal/testing/testhelper"
)

var (
	in      = htmlcheck.In
	hasText = htmlcheck.HasText
)

func TestModulePackageDirectoryResolution(t *testing.T) {
	// The shared test state sets up the following scenario to exercise the types
	// of problems discussed in b/143814014: a directory that becomes a package,
	// and then becomes a directory again. Specifically:
	//  + at v1.2.3, github.com/golang/found/dir is a directory (containing dir/pkg)
	//  + at v1.2.4, github.com/golang/found/dir is a package
	//  + at v1.2.5, github.com/golang/found/dir is again just a directory
	versions := []*proxy.TestVersion{
		proxy.NewTestVersion(t, "github.com/golang/found", "v1.2.3", map[string]string{
			"found.go":       "package found\nconst Value = 123",
			"dir/pkg/pkg.go": "package pkg\nconst Value = 321",
			"LICENSE":        testhelper.MITLicense,
		}),
		proxy.NewTestVersion(t, "github.com/golang/found", "v1.2.4", map[string]string{
			"found.go":       "package found\nconst Value = 124",
			"dir/pkg/pkg.go": "package pkg\nconst Value = 421",
			"dir/dir.go":     "package dir\nconst Value = \"I'm a package!\"",
			"LICENSE":        testhelper.MITLicense,
		}),
		proxy.NewTestVersion(t, "github.com/golang/found", "v1.2.5", map[string]string{
			"found.go":       "package found\nconst Value = 125",
			"dir/pkg/pkg.go": "package pkg\nconst Value = 521",
			"LICENSE":        testhelper.MITLicense,
		}),
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
			want:     in(".DetailsHeader", hasText("v1.2.5")),
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
				in(".DetailsContent", hasText("521"))),
		},
		{
			desc:     "earlier package",
			urlPath:  "/github.com/golang/found@v1.2.3/dir/pkg",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.3")),
				in(".DetailsContent", hasText("321"))),
		},
		// This test fails, because the fetchPackageOrModule logic instead returns
		// 404 and suggests a search for the later package.
		// TODO(b/143814014): uncomment once this bug is fixed.
		// {
		// 	desc:     "dir is initially a directory",
		// 	urlPath:  "/github.com/golang/found@v1.2.3/dir",
		// 	wantCode: http.StatusOK,
		// 	want:     in(".Directories", hasText("pkg")),
		// },
		{
			desc:     "dir becomes a package",
			urlPath:  "/github.com/golang/found@v1.2.4/dir",
			wantCode: http.StatusOK,
			want: in("",
				in(".DetailsHeader", hasText("v1.2.4")),
				in(".DetailsContent", hasText("I'm a package"))),
		},
		// This test fails, because fetchPackageOrMOdule again suggests a search
		// for the (now earlier) package.
		// TODO(b/143814014): uncomment once this bug is fixed.
		// {
		// 	desc:     "dir becomes a directory again",
		// 	urlPath:  "/github.com/golang/found@v1.2.5/dir",
		// 	wantCode: http.StatusOK,
		// 	want:     in(".Directories", hasText("pkg")),
		// },
		// This test fails, because we currently go to the latest version of
		// found/dir, which is v1.2.4. We should instead serve the directory from
		// the latest version of the module
		// TODO(b/143814014): uncomment once this bug is fixed.
		// {
		// 	desc:     "latest directory",
		// 	urlPath:  "/github.com/golang/found/dir",
		// 	wantCode: http.StatusOK,
		// 	want:     in(".Directories", hasText("pkg")),
		// },
	}
	s, err := frontend.NewServer(testDB, nil, "../../../content/static", false)
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

func processVersions(ctx context.Context, t *testing.T, testVersions []*proxy.TestVersion) {
	t.Helper()
	proxyClient, teardown := proxy.SetupTestProxy(t, testVersions)
	defer teardown()

	for _, tv := range testVersions {
		v, _, err := etl.FetchVersion(ctx, tv.ModulePath, tv.Version, proxyClient)
		if err != nil {
			t.Fatal(err)
		}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
}
