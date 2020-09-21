// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestExtractReadmesFromZip(t *testing.T) {
	stdlib.UseTestData = true

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	sortReadmes := func(readmes []*internal.Readme) {
		sort.Slice(readmes, func(i, j int) bool {
			return readmes[i].Filepath < readmes[j].Filepath
		})
	}

	for _, test := range []struct {
		modulePath, version string
		files               map[string]string
		want                []*internal.Readme
	}{
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.12.5",
			want: []*internal.Readme{
				{
					Filepath: "README.md",
					Contents: "# The Go Programming Language\n",
				},
				{
					Filepath: "cmd/pprof/README",
					Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
				},
			},
		},
		{
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			files: map[string]string{
				"README.md":  "README FILE FOR TESTING.",
				"foo/README": "Another README",
			},
			want: []*internal.Readme{
				{
					Filepath: "README.md",
					Contents: "README FILE FOR TESTING.",
				},
				{
					Filepath: "foo/README",
					Contents: "Another README",
				},
			},
		},
		{
			modulePath: "emp.ty/module",
			version:    "v1.0.0",
			files:      map[string]string{},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			var (
				reader *zip.Reader
				err    error
			)
			if test.modulePath == stdlib.ModulePath {
				reader, _, err = stdlib.Zip(test.version)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
					{ModulePath: test.modulePath, Files: test.files}})
				defer teardownProxy()
				reader, err = proxyClient.GetZip(ctx, test.modulePath, "v1.0.0")
				if err != nil {
					t.Fatal(err)
				}
			}

			got, err := extractReadmesFromZip(test.modulePath, test.version, reader)
			if err != nil {
				t.Fatal(err)
			}

			sortReadmes(test.want)
			sortReadmes(got)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsReadme(t *testing.T) {
	for _, test := range []struct {
		name, file string
		want       bool
	}{
		{
			name: "README in nested dir returns true",
			file: "github.com/my/module@v1.0.0/README.md",
			want: true,
		},
		{
			name: "case insensitive",
			file: "rEaDme",
			want: true,
		},
		{
			name: "random extension returns true",
			file: "README.FOO",
			want: true,
		},
		{
			name: "{prefix}readme will return false",
			file: "FOO_README",
			want: false,
		},
		{
			file: "README_FOO",
			name: "readme{suffix} will return false",
			want: false,
		},
		{
			file: "README.FOO.FOO",
			name: "README file with multiple extensions will return false",
			want: false,
		},
		{
			file: "readme.go",
			name: ".go README file will return false",
			want: false,
		},
		{
			file: "readme.vendor",
			name: ".vendor README file will return false",
			want: false,
		},
		{
			file: "",
			name: "empty filename returns false",
			want: false,
		},
	} {
		{
			t.Run(test.file, func(t *testing.T) {
				if got := isReadme(test.file); got != test.want {
					t.Errorf("isReadme(%q) = %t: %t", test.file, got, test.want)
				}
			})
		}
	}
}
