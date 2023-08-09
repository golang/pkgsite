// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package urlinfo

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

func TestExtractURLPathInfo(t *testing.T) {
	for _, test := range []struct {
		name, url string
		want      *URLPathInfo // nil => want non-nil error
	}{
		{
			name: "path at latest",
			url:  "/github.com/hashicorp/vault/api",
			want: &URLPathInfo{
				ModulePath:       internal.UnknownModulePath,
				FullPath:         "github.com/hashicorp/vault/api",
				RequestedVersion: version.Latest,
			},
		},
		{
			name: "path at version in nested module",
			url:  "/github.com/hashicorp/vault/api@v1.0.3",
			want: &URLPathInfo{
				ModulePath:       internal.UnknownModulePath,
				FullPath:         "github.com/hashicorp/vault/api",
				RequestedVersion: "v1.0.3",
			},
		},
		{
			name: "package at version in parent module",
			url:  "/github.com/hashicorp/vault@v1.0.3/api",
			want: &URLPathInfo{
				ModulePath:       "github.com/hashicorp/vault",
				FullPath:         "github.com/hashicorp/vault/api",
				RequestedVersion: "v1.0.3",
			},
		},
		{
			name: "package at version trailing slash",
			url:  "/github.com/hashicorp/vault/api@v1.0.3/",
			want: &URLPathInfo{
				ModulePath:       internal.UnknownModulePath,
				FullPath:         "github.com/hashicorp/vault/api",
				RequestedVersion: "v1.0.3",
			},
		},
		{
			name: "stdlib module",
			url:  "/std",
			want: &URLPathInfo{
				ModulePath:       stdlib.ModulePath,
				FullPath:         "std",
				RequestedVersion: version.Latest,
			},
		},
		{
			name: "stdlib module at version",
			url:  "/std@go1.14",
			want: &URLPathInfo{
				ModulePath:       stdlib.ModulePath,
				FullPath:         "std",
				RequestedVersion: "v1.14.0",
			},
		},
		{
			name: "stdlib",
			url:  "/net/http",
			want: &URLPathInfo{
				ModulePath:       stdlib.ModulePath,
				FullPath:         "net/http",
				RequestedVersion: version.Latest,
			},
		},
		{
			name: "stdlib at version",
			url:  "/net/http@go1.14",
			want: &URLPathInfo{
				ModulePath:       stdlib.ModulePath,
				FullPath:         "net/http",
				RequestedVersion: "v1.14.0",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ExtractURLPathInfo(test.url)
			if err != nil {
				t.Fatalf("ExtractURLPathInfo(%q): %v", test.url, err)
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(URLPathInfo{})); diff != "" {
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
			got, err := ExtractURLPathInfo(test.url)
			if (err != nil) != test.wantErr {
				t.Fatalf("ExtractURLPathInfo(%q) error = (%v); want error %t)", test.url, err, test.wantErr)
			}
			if !test.wantErr && (test.wantModulePath != got.ModulePath || test.wantVersion != got.RequestedVersion || test.wantFullPath != got.FullPath) {
				t.Fatalf("ExtractURLPathInfo(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					test.url, got.FullPath, got.ModulePath, got.RequestedVersion, err, test.wantFullPath, test.wantModulePath, test.wantVersion, test.wantErr)
			}
		})
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
		got := IsValidPath(test.path)
		if got != test.want {
			t.Errorf("IsValidPath(ctx, ds, %q) = %t, want %t", test.path, got, test.want)
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
		{"net/http", "v1.2.3", true}, // IsSupportedVersion expects the goTag is already converted to semver
		{"net/http", "v1.2.3.bad", false},
		{"net/http", "latest", true},
		{"net/http", "master", true},
		{"net/http", "main", false},
	}
	for _, test := range tests {
		got := IsSupportedVersion(test.path, test.version)
		if got != test.want {
			t.Errorf("IsSupportedVersion(ctx, ds, %q, %q) = %t, want %t", test.path, test.version, got, test.want)
		}
	}
}
