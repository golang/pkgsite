// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/sample"
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
			path: "github.com/foo/bar",
			want: []string{
				"github.com/foo",
				"github.com/foo/bar",
				"foo",
				"foo/bar",
				"bar",
			},
		},
		{
			path: "golang.org/x/tools/go/packages",
			want: []string{
				"go",
				"go/packages",
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
				"internal",
				"internal/valuecollector",
				"valuecollector",
			},
		},
		{
			path: "code.cloud.gitlab.google.k8s.io",
			want: []string{
				"cloud",
				"k8s",
				"code.cloud.gitlab.google.k8s.io",
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

func TestInsertSearchDocumentAndSearch(t *testing.T) {
	var (
		modGoCDK = "gocloud.dev"
		pkgGoCDK = &internal.Package{
			Name:     "cloud",
			Path:     "gocloud.dev/cloud",
			Synopsis: "Package cloud contains a library and tools for open cloud development in Go. The Go Cloud Development Kit (Go CDK)",
		}

		modKube = "k8s.io"
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
				CommitTime:  sample.CommitTime,
				Version:     sample.VersionString,
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
				CommitTime:  sample.CommitTime,
				Version:     sample.VersionString,
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
				modKube:  pkgKube,
				modGoCDK: pkgGoCDK,
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

			for modulePath, pkg := range tc.packages {
				pkg.Licenses = sample.LicenseMetadata
				v := sample.Version()
				v.ModulePath = modulePath
				v.Packages = []*internal.Package{pkg}
				if err := testDB.InsertVersion(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			if tc.limit < 1 {
				tc.limit = 10
			}

			got, err := testDB.Search(ctx, tc.searchQuery, tc.limit, tc.offset)
			if err != nil {
				t.Fatal(err)
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

func TestUpsertSearchDocumentVersionUpdatedAt(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	getVersionUpdatedAt := func(path string) time.Time {
		t.Helper()
		sd, err := testDB.getSearchDocument(ctx, path)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", path, err)
		}
		return sd.versionUpdatedAt
	}

	pkgA := &internal.Package{Path: "A", Name: "A"}
	mustInsertVersion := func(version string) {
		v := sample.Version()
		v.Packages = []*internal.Package{pkgA}
		v.Version = version
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	mustInsertVersion("v1.0.0")
	versionUpdatedAtOriginal := getVersionUpdatedAt(pkgA.Path)

	mustInsertVersion("v0.5.0")
	versionUpdatedAtNew := getVersionUpdatedAt(pkgA.Path)
	if versionUpdatedAtOriginal != versionUpdatedAtNew {
		t.Fatalf("expected version_updated_at to remain unchanged an older version was inserted; got versionUpdatedAtOriginal = %v; versionUpdatedAtNew = %v",
			versionUpdatedAtOriginal, versionUpdatedAtNew)
	}

	mustInsertVersion("v1.5.2")
	versionUpdatedAtNew2 := getVersionUpdatedAt(pkgA.Path)
	if versionUpdatedAtOriginal == versionUpdatedAtNew2 {
		t.Fatalf("expected version_updated_at to change since a newer version was inserted; got versionUpdatedAtNew = %v; versionUpdatedAtNew2 = %v",
			versionUpdatedAtOriginal, versionUpdatedAtNew)
	}
}

func TestUpdateSearchDocumentsImportedByCount(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	mustInsertPackageVersion := func(pkg *internal.Package, version string) {
		t.Helper()
		v := sample.Version()
		v.Packages = []*internal.Package{pkg}
		v.ModulePath = v.ModulePath + pkg.Path
		v.Version = version
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatalf("testDB.InsertVersionAndSearchDocuments(%q %q): %v", v.ModulePath, v.Version, err)
		}
	}
	mustUpdateImportedByCount := func() {
		t.Helper()
		if err := testDB.UpdateSearchDocumentsImportedByCount(ctx, 10); err != nil {
			t.Fatalf("testDB.UpdateSearchDocumentsImportedByCount(ctx, 10): %v", err)
		}
	}
	validateImportedByCountAndGetSearchDocument := func(path string, count int) *searchDocument {
		t.Helper()
		sd, err := testDB.getSearchDocument(ctx, path)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", path, err)
		}
		if sd.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("importedByCountUpdatedAt for package %q should not by empty", path)
		}
		if count != sd.importedByCount {
			t.Fatalf("importedByCount for package %q = %d; want = %d", path, sd.importedByCount, count)
		}
		return sd
	}

	// Test imported_by_count = 0 when only pkgA is added.
	pkgA := &internal.Package{Path: "A", Name: "A"}
	mustInsertPackageVersion(pkgA, "v1.0.0")
	mustUpdateImportedByCount()
	_ = validateImportedByCountAndGetSearchDocument(pkgA.Path, 0)

	// Test imported_by_count = 1 for pkgA when pkgB is added.
	pkgB := &internal.Package{Path: "B", Name: "B", Imports: []string{"A"}}
	mustInsertPackageVersion(pkgB, "v1.0.0")
	mustUpdateImportedByCount()
	_ = validateImportedByCountAndGetSearchDocument(pkgA.Path, 1)
	sdB := validateImportedByCountAndGetSearchDocument(pkgB.Path, 0)
	wantSearchDocBUpdatedAt := sdB.importedByCountUpdatedAt

	// Test imported_by_count = 2 for pkgA, when C is added.
	pkgC := &internal.Package{Path: "C", Name: "C", Imports: []string{"A"}}
	mustInsertPackageVersion(pkgC, "v1.0.0")
	mustUpdateImportedByCount()
	sdA := validateImportedByCountAndGetSearchDocument(pkgA.Path, 2)
	sdC := validateImportedByCountAndGetSearchDocument(pkgC.Path, 0)

	// Test imported_by_count_updated_at for A and C are the same.
	if sdA.importedByCountUpdatedAt != sdC.importedByCountUpdatedAt {
		t.Fatalf("expected imported_by_count_updated_at for pkgA and pkgC to be the same; pkgA = %v, pkgC = %v", sdA.importedByCountUpdatedAt, sdC.importedByCountUpdatedAt)
	}

	// Test imported_by_count_updated_at for B has not changed.
	sdB = validateImportedByCountAndGetSearchDocument(pkgB.Path, 0)
	if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
		t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v", wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
	}

	// Test imported_by_count_updated_at for B is before imported_by_count_updated_at for A.
	if !sdB.importedByCountUpdatedAt.Before(sdA.importedByCountUpdatedAt) {
		t.Fatalf("expected mported_by_count_updated_at for pkgB to be before pkgA; pkgB = %v, pkgA = %v", sdB.importedByCountUpdatedAt, sdA.importedByCountUpdatedAt)
	}

	// Test imported_by_count_updated_at for A and B have changed when a
	// newer version of B is added.
	mustInsertPackageVersion(pkgB, "v1.2.0")
	mustUpdateImportedByCount()
	sdA = validateImportedByCountAndGetSearchDocument(pkgA.Path, 2)
	sdB = validateImportedByCountAndGetSearchDocument(pkgB.Path, 0)
	if sdA.importedByCountUpdatedAt != sdB.importedByCountUpdatedAt {
		t.Fatalf("expected imported_by_count_updated_at for pkgA and pkgB to be the same; pkgA = %v, pkgB = %v", sdA.importedByCountUpdatedAt, sdB.importedByCountUpdatedAt)
	}

	// Test imported_by_count_updated_at for C is before imported_by_count_updated_at for A.
	_ = validateImportedByCountAndGetSearchDocument(pkgC.Path, 0)
	if !sdC.importedByCountUpdatedAt.Before(sdA.importedByCountUpdatedAt) {
		t.Fatalf("expected mported_by_count_updated_at for pkgC to be before pkgA; pkgB = %v, pkgA = %v", sdB.importedByCountUpdatedAt, sdA.importedByCountUpdatedAt)
	}

	// Test imported_by_count_updated_at for D changes when
	// an older version of A imports D.
	pkgD := &internal.Package{Path: "D", Name: "D"}
	mustInsertPackageVersion(pkgD, "v1.0.0")
	pkgA.Imports = []string{"D"}
	mustInsertPackageVersion(pkgA, "v0.9.0")
	mustUpdateImportedByCount()
	_ = validateImportedByCountAndGetSearchDocument(pkgA.Path, 2)
	_ = validateImportedByCountAndGetSearchDocument(pkgD.Path, 1)
}

func TestGetPackagesForSearchDocumentUpsert(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	versionA := sample.Version()
	versionA.Packages = []*internal.Package{
		{Path: "A", Name: "A"},
		{Path: "A/notinternal", Name: "A/notinternal"},
		{Path: "A/internal", Name: "A/internal"},
		{Path: "A/internal/B", Name: "A/internal/B"},
	}
	if err := testDB.saveVersion(ctx, versionA); err != nil {
		t.Fatal(err)
	}
	// pkgPaths should be "A", since pkg "A" exists in packages but not
	// search_documents.
	pkgPaths, err := testDB.GetPackagesForSearchDocumentUpsert(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A", "A/notinternal"}
	if diff := cmp.Diff(want, pkgPaths); diff != "" {
		t.Fatalf("testDB.GetPackagesForSearchDocumentUpsert mismatch(-want +got):\n%s", diff)
	}

	for _, path := range want {
		if err := testDB.UpsertSearchDocument(ctx, path); err != nil {
			t.Fatal(err)
		}
	}
	// pkgPaths should be an empty slice, since pkg "A" and "A/notinternal"
	// were just inserted into search_documents.
	pkgPaths, err = testDB.GetPackagesForSearchDocumentUpsert(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgPaths) != 0 {
		t.Fatalf("expected testDB.GetPackagesForSearchDocumentUpsert to return an empty slice; got %v", pkgPaths)
	}
}
