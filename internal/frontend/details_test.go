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
	"golang.org/x/discovery/internal/stdlib"
)

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
		err := checkPathAndVersion(context.Background(), fakeDataSource{}, test.path, test.version)
		var got int
		if err == nil {
			got = 200
		} else if serr, ok := err.(*serverError); ok {
			got = serr.status
		} else {
			got = -1
		}
		if got != test.want {
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
