// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestExtractURLPathInfo(t *testing.T) {
	for _, test := range []struct {
		name, url string
		want      *urlPathInfo // nil => want non-nil error
	}{
		{
			name: "path at latest",
			url:  "/github.com/hashicorp/vault/api",
			want: &urlPathInfo{
				modulePath:       internal.UnknownModulePath,
				fullPath:         "github.com/hashicorp/vault/api",
				requestedVersion: internal.LatestVersion,
			},
		},
		{
			name: "path at version in nested module",
			url:  "/github.com/hashicorp/vault/api@v1.0.3",
			want: &urlPathInfo{
				modulePath:       internal.UnknownModulePath,
				fullPath:         "github.com/hashicorp/vault/api",
				requestedVersion: "v1.0.3",
			},
		},
		{
			name: "package at version in parent module",
			url:  "/github.com/hashicorp/vault@v1.0.3/api",
			want: &urlPathInfo{
				modulePath:       "github.com/hashicorp/vault",
				fullPath:         "github.com/hashicorp/vault/api",
				requestedVersion: "v1.0.3",
			},
		},
		{
			name: "package at version trailing slash",
			url:  "/github.com/hashicorp/vault/api@v1.0.3/",
			want: &urlPathInfo{
				modulePath:       internal.UnknownModulePath,
				fullPath:         "github.com/hashicorp/vault/api",
				requestedVersion: "v1.0.3",
			},
		},
		{
			name: "stdlib module",
			url:  "/std",
			want: &urlPathInfo{
				modulePath:       stdlib.ModulePath,
				fullPath:         "std",
				requestedVersion: internal.LatestVersion,
			},
		},
		{
			name: "stdlib module at version",
			url:  "/std@go1.14",
			want: &urlPathInfo{
				modulePath:       stdlib.ModulePath,
				fullPath:         "std",
				requestedVersion: "v1.14.0",
			},
		},
		{
			name: "stdlib",
			url:  "/net/http",
			want: &urlPathInfo{
				modulePath:       stdlib.ModulePath,
				fullPath:         "net/http",
				requestedVersion: internal.LatestVersion,
			},
		},
		{
			name: "stdlib at version",
			url:  "/net/http@go1.14",
			want: &urlPathInfo{
				modulePath:       stdlib.ModulePath,
				fullPath:         "net/http",
				requestedVersion: "v1.14.0",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractURLPathInfo(test.url)
			if err != nil {
				t.Fatalf("extractURLPathInfo(%q): %v", test.url, err)
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(urlPathInfo{})); diff != "" {
				t.Errorf("%q: mismatch (-want, +got):\n%s", test.url, diff)
			}
		})
	}
}

func TestExtractURLPathInfo_Errors(t *testing.T) {
	testCases := []struct {
		name, url, wantModulePath, wantFullPath, wantVersion string
		wantErr                                              bool
	}{
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
			got, err := extractURLPathInfo(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("extractURLPathInfo(%q) error = (%v); want error %t)", tc.url, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModulePath != got.modulePath || tc.wantVersion != got.requestedVersion || tc.wantFullPath != got.fullPath) {
				t.Fatalf("extractURLPathInfo(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					tc.url, got.fullPath, got.modulePath, got.requestedVersion, err, tc.wantFullPath, tc.wantModulePath, tc.wantVersion, tc.wantErr)
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

func TestNewContextFromExps(t *testing.T) {
	for _, test := range []struct {
		mods []string
		want []string
	}{
		{
			mods: []string{"c", "a", "b"},
			want: []string{"a", "b", "c"},
		},
		{
			mods: []string{"d", "a"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			mods: []string{"d", "!b", "!a", "c"},
			want: []string{"c", "d"},
		},
	} {
		ctx := experiment.NewContext(context.Background(), "a", "b", "c")
		ctx = newContextFromExps(ctx, test.mods)
		got := experiment.FromContext(ctx).Active()
		sort.Strings(got)
		if !cmp.Equal(got, test.want) {
			t.Errorf("mods=%v:\ngot  %v\nwant %v", test.mods, got, test.want)
		}
	}
}
