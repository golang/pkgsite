// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"io/fs"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestExtractReadmes(t *testing.T) {
	defer stdlib.WithTestData()()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	sortReadmes := func(readmes []*internal.Readme) {
		sort.Slice(readmes, func(i, j int) bool {
			return readmes[i].Filepath < readmes[j].Filepath
		})
	}

	for _, test := range []struct {
		name, modulePath, version string
		files                     map[string]string
		want                      []*internal.Readme
	}{
		{
			name:       "README at root and README in unit",
			modulePath: stdlib.ModulePath,
			version:    "v1.12.5",
			want: []*internal.Readme{
				{
					Filepath: "cmd/pprof/README",
					Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
				},
			},
		},
		{
			name:       "directory start with _",
			modulePath: stdlib.ModulePath,
			version:    "v1.12.5",
			want: []*internal.Readme{
				{
					Filepath: "cmd/pprof/README",
					Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
				},
			},
		},
		{
			name:       "prefer README.md",
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			files: map[string]string{
				"foo/README":    "README",
				"foo/README.md": "README",
			},
			want: []*internal.Readme{
				{
					Filepath: "foo/README.md",
					Contents: "README",
				},
			},
		},
		{
			name:       "prefer readme.markdown",
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			files: map[string]string{
				"foo/README.markdown": "README",
				"foo/readme.rst":      "README",
			},
			want: []*internal.Readme{
				{
					Filepath: "foo/README.markdown",
					Contents: "README",
				},
			},
		},
		{
			name:       "no readme",
			modulePath: "emp.ty/module",
			version:    "v1.0.0",
			files:      map[string]string{},
		},
		{
			name:       "readme is a directory",
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			files: map[string]string{
				"foo/README/bar": "README",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var (
				contentDir fs.FS
				err        error
			)
			if test.modulePath == stdlib.ModulePath {
				contentDir, _, _, err = stdlib.ContentDir(test.version)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				proxyClient, teardownProxy := proxytest.SetupTestClient(t, []*proxytest.Module{
					{ModulePath: test.modulePath, Files: test.files}})
				defer teardownProxy()
				reader, err := proxyClient.Zip(ctx, test.modulePath, "v1.0.0")
				if err != nil {
					t.Fatal(err)
				}
				contentDir, err = fs.Sub(reader, test.modulePath+"@v1.0.0")
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := extractReadmes(test.modulePath, test.version, contentDir)
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
