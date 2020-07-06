// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestExtractURLPathInfo(t *testing.T) {
	for _, test := range []struct {
		in   string
		want *urlPathInfo // nil => want non-nil error
	}{
		{"", nil},
		{
			"/a.com",
			&urlPathInfo{
				fullPath:         "a.com",
				modulePath:       internal.UnknownModulePath,
				requestedVersion: internal.LatestVersion,
				isModule:         false,
				urlPath:          "/a.com",
			},
		},
		{
			"/a.com@v1.2.3",
			&urlPathInfo{
				fullPath:         "a.com",
				modulePath:       internal.UnknownModulePath,
				requestedVersion: "v1.2.3",
				isModule:         false,
				urlPath:          "/a.com@v1.2.3",
			},
		},
		{
			"/a.com@v1.2.3/b",
			&urlPathInfo{
				fullPath:         "a.com/b",
				modulePath:       "a.com",
				requestedVersion: "v1.2.3",
				isModule:         false,
				urlPath:          "/a.com@v1.2.3/b",
			},
		},
		{
			"/encoding/json",
			&urlPathInfo{
				fullPath:         "encoding/json",
				modulePath:       "std",
				requestedVersion: internal.LatestVersion,
				isModule:         false,
				urlPath:          "/encoding/json",
			},
		},
		{
			"/encoding/json@go1.12",
			&urlPathInfo{
				fullPath:         "encoding/json",
				modulePath:       "std",
				requestedVersion: "v1.12.0",
				isModule:         false,
				urlPath:          "/encoding/json@go1.12",
			},
		},
		{
			"/mod/a.com",
			&urlPathInfo{
				fullPath:         "a.com",
				modulePath:       internal.UnknownModulePath,
				requestedVersion: internal.LatestVersion,
				isModule:         true,
				urlPath:          "/a.com",
			},
		},
		{
			"/mod/a.com@v1.2.3",
			&urlPathInfo{
				fullPath:         "a.com",
				modulePath:       internal.UnknownModulePath,
				requestedVersion: "v1.2.3",
				isModule:         true,
				urlPath:          "/a.com@v1.2.3",
			},
		},
	} {
		got, err := extractURLPathInfo(test.in)
		if err != nil {
			if test.want != nil {
				t.Errorf("%q: got error %v", test.in, err)
			}
			continue
		}
		if test.want == nil {
			t.Errorf("%q: got no error, wanted one", test.in)
			continue
		}
		if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(urlPathInfo{})); diff != "" {
			t.Errorf("%q: mismatch (-want, +got):\n%s", test.in, diff)
		}
	}
}

func TestParseDetailsURLPath(t *testing.T) {
	testCases := []struct {
		name, url, wantModulePath, wantFullPath, wantVersion string
		wantErr                                              bool
	}{
		{
			name:           "latest",
			url:            "/github.com/hashicorp/vault/api",
			wantModulePath: internal.UnknownModulePath,
			wantFullPath:   "github.com/hashicorp/vault/api",
			wantVersion:    internal.LatestVersion,
		},
		{
			name:           "package at version in nested module",
			url:            "/github.com/hashicorp/vault/api@v1.0.3",
			wantModulePath: internal.UnknownModulePath,
			wantFullPath:   "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version in parent module",
			url:            "/github.com/hashicorp/vault@v1.0.3/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantFullPath:   "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version trailing slash",
			url:            "/github.com/hashicorp/vault/api@v1.0.3/",
			wantModulePath: internal.UnknownModulePath,
			wantFullPath:   "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "stdlib",
			url:            "net/http",
			wantModulePath: stdlib.ModulePath,
			wantFullPath:   "net/http",
			wantVersion:    internal.LatestVersion,
		},
		{
			name:           "stdlib at version",
			url:            "net/http@go1.14",
			wantModulePath: stdlib.ModulePath,
			wantFullPath:   "net/http",
			wantVersion:    "go1.14",
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
		{
			name:    "explicit latest",
			url:     "/github.com/hashicorp/vault/api@latest",
			wantErr: true,
		},
		{
			name:    "split stdlib",
			url:     "/net@go1.14/http",
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
			if !tc.wantErr && (tc.wantModulePath != gotModule || tc.wantVersion != gotVersion || tc.wantFullPath != gotPkg) {
				t.Fatalf("parseDetailsURLPath(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					u, gotPkg, gotModule, gotVersion, err, tc.wantFullPath, tc.wantModulePath, tc.wantVersion, tc.wantErr)
			}
		})
	}
}

func TestValidatePathAndVersion(t *testing.T) {
	tests := []struct {
		path, version string
		want          int
	}{
		{"import/path", "v1.2.3", http.StatusOK},
		{"import/path", "v1.2.bad", http.StatusBadRequest},
	}

	for _, test := range tests {
		err := validatePathAndVersion(context.Background(), fakeDataSource{}, test.path, test.version)
		var got int
		if err == nil {
			got = 200
		} else if serr, ok := err.(*serverError); ok {
			got = serr.status
		} else {
			got = -1
		}
		if got != test.want {
			t.Errorf("validatePathAndVersion(ctx, ds, %q, %q): got code %d, want %d", test.path, test.version, got, test.want)
		}
	}
}

type fakeDataSource struct {
	internal.DataSource
}
