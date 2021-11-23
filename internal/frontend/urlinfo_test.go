// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
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
				requestedVersion: version.Latest,
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
				requestedVersion: version.Latest,
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
				requestedVersion: version.Latest,
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
			name:    "invalid url for github.com",
			url:     "/github.com/foo",
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
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractURLPathInfo(test.url)
			if (err != nil) != test.wantErr {
				t.Fatalf("extractURLPathInfo(%q) error = (%v); want error %t)", test.url, err, test.wantErr)
			}
			if !test.wantErr && (test.wantModulePath != got.modulePath || test.wantVersion != got.requestedVersion || test.wantFullPath != got.fullPath) {
				t.Fatalf("extractURLPathInfo(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					test.url, got.fullPath, got.modulePath, got.requestedVersion, err, test.wantFullPath, test.wantModulePath, test.wantVersion, test.wantErr)
			}
		})
	}
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

func TestIsValidPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"net/http", true},
		{"github.com/foo", false},
		{"github.com/foo", false},
		{"/github.com/foo/bar", false},
		{"github.com/foo/bar/", false},
		{"github.com/foo/bar", true},
		{"github.com/foo/bar/baz", true},
		{"golang.org/dl", true},
		{"golang.org/dl/go1.2.3", true},
		{"golang.org/x", false},
		{"golang.org/x/tools", true},
		{"golang.org/x/tools/go/packages", true},
		{"gopkg.in/yaml.v2", true},
	}
	for _, test := range tests {
		got := isValidPath(test.path)
		if got != test.want {
			t.Errorf("isValidPath(ctx, ds, %q) = %t, want %t", test.path, got, test.want)
		}
	}
}

func TestIsSupportedVersion(t *testing.T) {
	tests := []struct {
		path, version string
		want          bool
	}{
		{sample.ModulePath, "v1.2.3", true},
		{sample.ModulePath, "v1.2.bad", false},
		{sample.ModulePath, "latest", true},
		{sample.ModulePath, "master", true},
		{sample.ModulePath, "main", true},
		{"net/http", "v1.2.3", true}, // isSupportedVersion expects the goTag is already converted to semver
		{"net/http", "v1.2.3.bad", false},
		{"net/http", "latest", true},
		{"net/http", "master", true},
		{"net/http", "main", false},
	}
	for _, test := range tests {
		got := isSupportedVersion(test.path, test.version)
		if got != test.want {
			t.Errorf("isSupportedVersion(ctx, ds, %q, %q) = %t, want %t", test.path, test.version, got, test.want)
		}
	}
}
