// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
)

func TestPathTokens(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{
			path: "context",
			want: []string{"context"},
		},
		{
			path: "rsc.io/quote",
			want: []string{
				"quote",
				"rsc",
				"rsc.io",
				"rsc.io/quote",
			},
		},
		{
			path: "k8s.io/client-go",
			want: []string{
				"k8s",
				"k8s.io",
				"client",
				"go",
				"client-go",
				"k8s.io/client-go",
			},
		},
		{
			path: "golang.org/x/tools/go/packages",
			want: []string{
				"go",
				"go/packages",
				"golang",
				"golang.org",
				"golang.org/x",
				"golang.org/x/tools",
				"golang.org/x/tools/go",
				"golang.org/x/tools/go/packages",
				"packages",
				"tools",
				"tools/go",
				"tools/go/packages",
				"x",
				"x/tools",
				"x/tools/go",
				"x/tools/go/packages",
			},
		},
		{
			path: "/example.com/foo-bar///package///",
			want: []string{
				"bar",
				"example",
				"example.com",
				"example.com/foo-bar",
				"example.com/foo-bar///package",
				"foo",
				"foo-bar",
				"foo-bar///package",
				"package",
			},
		},
		{
			path: "cloud.google.com/go/cmd/go-cloud-debug-agent/internal/valuecollector",
			want: []string{
				"agent",
				"cloud",
				"cloud.google.com",
				"cloud.google.com/go",
				"cloud.google.com/go/cmd",
				"cloud.google.com/go/cmd/go-cloud-debug-agent",
				"cloud.google.com/go/cmd/go-cloud-debug-agent/internal",
				"cloud.google.com/go/cmd/go-cloud-debug-agent/internal/valuecollector",
				"cmd",
				"cmd/go-cloud-debug-agent",
				"cmd/go-cloud-debug-agent/internal",
				"cmd/go-cloud-debug-agent/internal/valuecollector",
				"debug",
				"go",
				"go-cloud-debug-agent",
				"go-cloud-debug-agent/internal",
				"go-cloud-debug-agent/internal/valuecollector",
				"go/cmd",
				"go/cmd/go-cloud-debug-agent",
				"go/cmd/go-cloud-debug-agent/internal",
				"go/cmd/go-cloud-debug-agent/internal/valuecollector",
				"google",
				"internal",
				"internal/valuecollector",
				"valuecollector",
			},
		},
		{
			path: "/",
			want: nil,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := generatePathTokens(tc.path)
			sort.Strings(got)
			sort.Strings(tc.want)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("generatePathTokens(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

// insertPackage creates and inserts a version using sampleVersion, that has
// only the package pkg. It is a helper function for
// TestInsertDocumentsAndSearch.
func insertPackage(ctx context.Context, t *testing.T, modulePath string, pkg *internal.Package) {
	t.Helper()

	v := sampleVersion(func(v *internal.Version) {
		v.ModulePath = modulePath
		pkg.Licenses = sampleLicenseInfos
		v.Packages = []*internal.Package{pkg}
	})
	if err := testDB.InsertVersion(ctx, v, sampleLicenses); err != nil {
		t.Fatalf("testDB.InsertVersion(%+v): %v", v, err)
	}
	if err := testDB.InsertDocuments(ctx, v); err != nil {
		t.Fatalf("testDB.InsertDocument(%+v): %v", v, err)
	}
	if err := testDB.RefreshSearchDocuments(ctx); err != nil {
		t.Fatalf("testDB.RefreshSearchDocuments(ctx): %v", err)
	}
}

func TestInsertDocumentsAndSearch(t *testing.T) {
	var (
		modGoCDK = "my.mod/cdk"
		pkgGoCDK = &internal.Package{
			Name:     "cloud",
			Path:     "gocloud.dev",
			Synopsis: "Package cloud contains a library and tools for open cloud development in Go. The Go Cloud Development Kit (Go CDK)",
		}

		modKube = "my.mod/kube"
		pkgKube = &internal.Package{
			Name:     "client-go",
			Path:     "k8s.io/client-go",
			Synopsis: "Package client-go implements a Go client for Kubernetes.",
		}

		kubeResult = func(rank float64, numResults uint64) *SearchResult {
			return &SearchResult{
				Name:        pkgKube.Name,
				PackagePath: pkgKube.Path,
				Synopsis:    pkgKube.Synopsis,
				Licenses:    []string{"MIT"},
				CommitTime:  now,
				Version:     sampleVersionString,
				ModulePath:  modKube,
				Rank:        rank,
				NumResults:  numResults,
			}
		}

		goCdkResult = func(rank float64, numResults uint64) *SearchResult {
			return &SearchResult{
				Name:        pkgGoCDK.Name,
				PackagePath: pkgGoCDK.Path,
				Synopsis:    pkgGoCDK.Synopsis,
				Licenses:    []string{"MIT"},
				CommitTime:  now,
				Version:     sampleVersionString,
				ModulePath:  modGoCDK,
				Rank:        rank,
				NumResults:  numResults,
			}
		}
	)

	for _, tc := range []struct {
		name          string
		packages      map[string]*internal.Package
		limit, offset int
		searchQuery   string
		want          []*SearchResult
	}{
		{
			name:        "two documents, single term search",
			searchQuery: "package",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*SearchResult{
				goCdkResult(0.10560775506982405, 2),
				kubeResult(0.10560775506982405, 2),
			},
		},
		{
			name:        "two documents, single term search, two results limit 1 offset 0",
			limit:       1,
			offset:      0,
			searchQuery: "package",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*SearchResult{
				goCdkResult(0.10560775506982405, 2),
			},
		},
		{
			name:        "two documents, single term search, two results limit 1 offset 1",
			limit:       1,
			offset:      1,
			searchQuery: "package",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*SearchResult{
				kubeResult(0.10560775506982405, 2),
			},
		},
		{
			name:        "two documents, multiple term search",
			searchQuery: "go & cdk",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*SearchResult{
				goCdkResult(0.3187147723292191, 1),
			},
		},
		{
			name:        "one document, single term search",
			searchQuery: "cloud",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
			},
			want: []*SearchResult{
				goCdkResult(0.30875602614034653, 1),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for m, p := range tc.packages {
				insertPackage(ctx, t, m, p)
			}

			if tc.limit < 1 {
				tc.limit = 10
			}

			got, err := testDB.Search(ctx, tc.searchQuery, tc.limit, tc.offset)
			if err != nil {
				t.Fatalf("testDB.Search(%v, %d, %d): %v", tc.searchQuery, tc.limit, tc.offset, err)
			}

			if len(got) != len(tc.want) {
				t.Errorf("testDB.Search(%v, %d, %d) mismatch: len(got) = %d, want = %d\n", tc.searchQuery, tc.limit, tc.offset, len(got), len(tc.want))
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("testDB.Search(%v, %d, %d) mismatch (-want +got):\n%s", tc.searchQuery, tc.limit, tc.offset, diff)
			}
		})
	}
}
