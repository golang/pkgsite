// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
)

func TestParseDetailsURLPath(t *testing.T) {
	testCases := []struct {
		name, url, wantModulePath, wantPkgPath, wantVersion string
		wantErr                                             bool
	}{
		{
			name:           "latest",
			url:            "/github.com/hashicorp/vault/api",
			wantModulePath: internal.UnknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    internal.LatestVersion,
		},
		{
			name:           "package at version in nested module",
			url:            "/github.com/hashicorp/vault/api@v1.0.3",
			wantModulePath: internal.UnknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version in parent module",
			url:            "/github.com/hashicorp/vault@v1.0.3/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version trailing slash",
			url:            "/github.com/hashicorp/vault/api@v1.0.3/",
			wantModulePath: internal.UnknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:    "invalid url",
			url:     "/",
			wantErr: true,
		},
		{
			name:    "invalid url missing module",
			url:     "@v1.0.0",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			gotPkg, gotModule, gotVersion, err := parseDetailsURLPath(u.Path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseDetailsURLPath(%q) error = (%v); want error %t)", u, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModulePath != gotModule || tc.wantVersion != gotVersion || tc.wantPkgPath != gotPkg) {
				t.Fatalf("parseDetailsURLPath(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					u, gotPkg, gotModule, gotVersion, err, tc.wantPkgPath, tc.wantModulePath, tc.wantVersion, tc.wantErr)
			}
		})
	}
}

func TestProcessPackageOrModulePath(t *testing.T) {
	for _, tc := range []struct {
		desc             string
		urlPath          string
		getErr1, getErr2 error

		wantPath, wantVersion string
		wantCode              int
	}{
		{
			desc:        "specific version found",
			urlPath:     "import/path@v1.2.3",
			wantPath:    "import/path",
			wantVersion: "v1.2.3",
			wantCode:    http.StatusOK,
		},
		{
			desc:        "latest version found",
			urlPath:     "import/path",
			wantPath:    "import/path",
			wantVersion: "latest",
			wantCode:    http.StatusOK,
		},
		{
			desc:        "version failed",
			urlPath:     "import/path@v1.2.3",
			getErr1:     context.Canceled,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusInternalServerError,
		},
		{
			desc:        "version not found, latest found",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     nil,
			wantPath:    "import/path",
			wantVersion: "v1.2.3",
			wantCode:    http.StatusSeeOther,
		},
		{
			desc:        "version not found, latest not found",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     derrors.NotFound,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusNotFound,
		},
		{
			desc:        "version not found, latest error",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     context.Canceled,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusNotFound,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ncalls := 0
			get := func(v string) (string, error) {
				ncalls++
				if ncalls == 1 {
					return "", tc.getErr1
				}
				return "", tc.getErr2
			}

			pkgPath, _, version, err := parseDetailsURLPath(tc.urlPath)
			if err != nil {
				t.Fatal(err)
			}
			gotCode, _ := fetchPackageOrModule(context.Background(), "pkg", pkgPath, version, get)
			if gotCode != tc.wantCode {
				t.Fatalf("got status code %d, want %d", gotCode, tc.wantCode)
			}
		})
	}
}

func TestCheckPathAndVersion(t *testing.T) {
	tests := []struct {
		path, version string
		want          int
	}{
		{"import/path", "v1.2.3", http.StatusOK},
		{"bad/path", "v1.2.3", http.StatusNotFound},
		{"import/path", "v1.2.bad", http.StatusBadRequest},
	}

	for _, test := range tests {
		if got, _ := checkPathAndVersion(context.Background(), fakeDataSource{}, test.path, test.version); got != test.want {
			t.Errorf("checkPathAndVersion(ctx, ds, %q, %q): got code %d, want %d", test.path, test.version, got, test.want)
		}
	}
}

type fakeDataSource struct {
	internal.DataSource
}

func (fakeDataSource) IsExcluded(_ context.Context, path string) (bool, error) {
	return strings.HasPrefix(path, "bad"), nil
}
