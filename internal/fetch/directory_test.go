// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestDirectoryPaths(t *testing.T) {
	for _, test := range []struct {
		name, modulePath string
		packagePaths     []string
		want             []string
	}{
		{
			name:       "no packages",
			modulePath: "github.com/empty/module",
			want:       []string{"github.com/empty/module"},
		},
		{
			name:         "only root package",
			modulePath:   "github.com/russross/blackfriday",
			packagePaths: []string{"github.com/russross/blackfriday"},
			want:         []string{"github.com/russross/blackfriday"},
		},
		{
			name:       "multiple packages and directories",
			modulePath: "github.com/elastic/go-elasticsearch/v7",
			packagePaths: []string{
				"github.com/elastic/go-elasticsearch/v7/esapi",
				"github.com/elastic/go-elasticsearch/v7/estransport",
				"github.com/elastic/go-elasticsearch/v7/esutil",
				"github.com/elastic/go-elasticsearch/v7/internal/version",
			},
			want: []string{
				"github.com/elastic/go-elasticsearch/v7",
				"github.com/elastic/go-elasticsearch/v7/esapi",
				"github.com/elastic/go-elasticsearch/v7/estransport",
				"github.com/elastic/go-elasticsearch/v7/esutil",
				"github.com/elastic/go-elasticsearch/v7/internal",
				"github.com/elastic/go-elasticsearch/v7/internal/version",
			},
		},
		{
			name:         "std lib",
			modulePath:   stdlib.ModulePath,
			packagePaths: []string{"cmd/go"},
			want:         []string{"cmd", "cmd/go", "std"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var packages []*internal.Package
			for _, path := range test.packagePaths {
				pkg := sample.Package()
				pkg.Path = path
				packages = append(packages, pkg)
			}
			got := directoryPaths(test.modulePath, packages)
			sort.Strings(got)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("directoryPaths(%q, %q)  mismatch (-want +got):\n%s",
					test.modulePath, test.packagePaths, diff)
			}
		})
	}
}
