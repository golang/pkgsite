// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opencensus.io/stats/view"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/testing/sample"
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

func TestSearch(t *testing.T) {
	// importGraph constructs a simple import graph where all importers import
	// one popular package.  For performance purposes, all importers are added to
	// a single importing module.
	importGraph := func(popularPath, importerModule string, importerCount int) []*internal.Version {
		v := sample.Version()
		v.ModulePath = popularPath
		v.Packages[0].Path = popularPath
		v.Packages[0].Imports = nil
		// Try to improve the ts_rank of the 'foo' search term.
		v.Packages[0].Synopsis = "foo"
		v.ReadmeContents = "foo"
		vers := []*internal.Version{v}
		if importerCount > 0 {
			v := sample.Version()
			v.ModulePath = importerModule
			v.Packages = nil
			for i := 0; i < importerCount; i++ {
				p := sample.Package()
				p.Path = fmt.Sprintf("%s/importer%d", importerModule, i)
				p.Imports = []string{popularPath}
				v.Packages = append(v.Packages, p)
			}
			vers = append(vers, v)
		}
		return vers
	}
	tests := []struct {
		label       string
		versions    []*internal.Version
		resultOrder [4]string
		wantSource  string
		wantResults []string
		wantTotal   uint64
	}{
		{
			label:       "single package",
			versions:    importGraph("foo.com/A", "", 0),
			resultOrder: [4]string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/A"},
			wantTotal:   1,
		},
		{
			label:       "empty results",
			versions:    []*internal.Version{},
			resultOrder: [4]string{"deep", "estimate", "popular"},
			wantSource:  "deep",
			wantResults: nil,
		},
		{
			label:       "both popular and unpopular results",
			versions:    importGraph("foo.com/popular", "bar.com/foo", 10),
			resultOrder: [4]string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popular", "bar.com/foo/importer0"},
			wantTotal:   11, // HLL result count (happens to be right in this case)
		},
		{
			label: "popular results, estimate before deep",
			versions: append(importGraph("foo.com/popularA", "bar.com", 60),
				importGraph("foo.com/popularB", "baz.com/foo", 70)...),
			resultOrder: [4]string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popularB", "foo.com/popularA"},
			wantTotal:   76, // HLL result count (actual count is 72)
		},
		{
			label: "popular results, deep before estimate",
			versions: append(importGraph("foo.com/popularA", "bar.com/foo", 60),
				importGraph("foo.com/popularB", "bar.com/foo", 70)...),
			resultOrder: [4]string{"popular", "deep", "estimate"},
			wantSource:  "deep",
			wantResults: []string{"foo.com/popularB", "foo.com/popularA"},
			wantTotal:   72,
		},
		// Adding a test for *very* popular results requires ~300 importers
		// minimum, which is pretty slow to set up at the moment (~5 seconds), and
		// doesn't add much additional value.
	}

	view.Register(SearchResponseCount)
	defer view.Unregister(SearchResponseCount)
	responses := make(map[string]int64)
	// responseDelta captures the change in the SearchResponseCount metric.
	responseDelta := func() map[string]int64 {
		rows, err := view.RetrieveData(SearchResponseCount.Name)
		if err != nil {
			t.Fatal(err)
		}
		delta := make(map[string]int64)
		for _, row := range rows {
			// Tags[0] should always be the SearchSource. For simplicity we assume
			// this.
			source := row.Tags[0].Value
			count := row.Data.(*view.CountData).Value
			if d := count - responses[source]; d != 0 {
				delta[source] = d
			}
			responses[source] = count
		}
		return delta
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			// use guardTestResult to simulate a scenario where search queries return
			// in the order: popular-50, popular-8, deep. In practice this should
			// usually be the order in which they return as these searches represent
			// progressively larger scans, but in our small test database this need
			// not be the case.
			done := make(map[string](chan struct{}))
			// waitFor maps [search type] -> [the search type is should wait for]
			waitFor := make(map[string]string)
			for i := 0; i < len(test.resultOrder); i++ {
				done[test.resultOrder[i]] = make(chan struct{})
				if i > 0 {
					waitFor[test.resultOrder[i]] = test.resultOrder[i-1]
				}
			}
			guardTestResult := func(source string) func() {
				// This test is inherently racy as 'estimate' results are are on a
				// separate channel, and therefore even after guarding still race to
				// the select statement.
				//
				// Since this is a concern only for testing, and since this test is
				// rather slow anyway, just wait for a healthy amount of time in order
				// to de-flake the test. If the test still proves flaky, we can either
				// increase this sleep or refactor so that all asynchronous results
				// arrive on the same channel.
				if source == "estimate" {
					time.Sleep(100 * time.Millisecond)
				}
				if await, ok := waitFor[source]; ok {
					<-done[await]
				}
				return func() { close(done[source]) }
			}
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, v := range test.versions {
				if err := testDB.InsertVersion(ctx, v); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
				t.Fatal(err)
			}
			results, err := testDB.hedgedSearch(ctx, "foo", 2, 0, searchers, guardTestResult)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range results {
				got = append(got, r.PackagePath)
			}
			if diff := cmp.Diff(test.wantResults, got); diff != "" {
				t.Errorf("FastSearch(\"foo\") mismatch (-want +got)\n%s", diff)
			}
			if len(results) > 0 && results[0].NumResults != test.wantTotal {
				t.Errorf("NumResults = %d, want %d", results[0].NumResults, test.wantTotal)
			}
			// Finally, validate that metrics are updated correctly
			gotDelta := responseDelta()
			wantDelta := map[string]int64{test.wantSource: 1}
			if diff := cmp.Diff(wantDelta, gotDelta); diff != "" {
				t.Errorf("SearchResponseCount: unexpected delta (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInsertSearchDocumentAndSearch(t *testing.T) {
	var (
		modGoCDK = "gocloud.dev"
		pkgGoCDK = &internal.Package{
			Name:              "cloud",
			Path:              "gocloud.dev/cloud",
			Synopsis:          "Package cloud contains a library and tools for open cloud development in Go. The Go Cloud Development Kit (Go CDK)",
			IsRedistributable: true, // required because some test cases depend on the README contents
		}

		modKube = "k8s.io"
		pkgKube = &internal.Package{
			Name:              "client-go",
			Path:              "k8s.io/client-go",
			Synopsis:          "Package client-go implements a Go client for Kubernetes.",
			IsRedistributable: true, // required because some test cases depend on the README contents
		}

		kubeResult = func(score float64, numResults uint64) *internal.SearchResult {
			return &internal.SearchResult{
				Name:        pkgKube.Name,
				PackagePath: pkgKube.Path,
				Synopsis:    pkgKube.Synopsis,
				Licenses:    []string{"MIT"},
				CommitTime:  sample.CommitTime,
				Version:     sample.VersionString,
				ModulePath:  modKube,
				Score:       score,
				NumResults:  numResults,
			}
		}

		goCdkResult = func(score float64, numResults uint64) *internal.SearchResult {
			return &internal.SearchResult{
				Name:        pkgGoCDK.Name,
				PackagePath: pkgGoCDK.Path,
				Synopsis:    pkgGoCDK.Synopsis,
				Licenses:    []string{"MIT"},
				CommitTime:  sample.CommitTime,
				Version:     sample.VersionString,
				ModulePath:  modGoCDK,
				Score:       score,
				NumResults:  numResults,
			}
		}
	)

	for _, tc := range []struct {
		name          string
		packages      map[string]*internal.Package
		limit, offset int
		searchQuery   string
		want          []*internal.SearchResult
	}{
		{
			name:        "two documents, single term search",
			searchQuery: "package",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*internal.SearchResult{
				goCdkResult(0.2431708425283432, 2),
				kubeResult(0.2431708425283432, 2),
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
			want: []*internal.SearchResult{
				goCdkResult(0.2431708425283432, 2),
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
			want: []*internal.SearchResult{
				kubeResult(0.2431708425283432, 2),
			},
		},
		{
			name:        "two documents, multiple term search",
			searchQuery: "go & cdk",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*internal.SearchResult{
				goCdkResult(0.733867883682251, 1),
			},
		},
		{
			name:        "one document, single term search",
			searchQuery: "cloud",
			packages: map[string]*internal.Package{
				modGoCDK: pkgGoCDK,
			},
			want: []*internal.SearchResult{
				goCdkResult(0.7109370231628418, 1),
			},
		},
	} {
		for method, searcher := range searchers {
			t.Run(tc.name+":"+method, func(t *testing.T) {
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

				got := searcher(testDB, ctx, tc.searchQuery, tc.limit, tc.offset)
				if got.err != nil {
					t.Fatal(got.err)
				}
				// Normally done by hedgedSearch, but we're bypassing that.
				if err := testDB.addPackageDataToSearchResults(ctx, got.results); err != nil {
					t.Fatal(err)
				}
				if len(got.results) != len(tc.want) {
					t.Errorf("testDB.Search(%v, %d, %d) mismatch: len(got) = %d, want = %d\n", tc.searchQuery, tc.limit, tc.offset, len(got.results), len(tc.want))
				}

				// The searchers differ in these two fields.
				opt := cmpopts.IgnoreFields(internal.SearchResult{}, "Approximate", "NumResults")
				if diff := cmp.Diff(tc.want, got.results, opt); diff != "" {
					t.Errorf("testDB.Search(%v, %d, %d) mismatch (-want +got):\n%s", tc.searchQuery, tc.limit, tc.offset, diff)
				}
			})
		}
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

func TestUpsertSearchDocumentVersionHasGoMod(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, hasGoMod := range []bool{true, false} {
		v := sample.Version()
		v.ModulePath = fmt.Sprintf("foo.com/%t", hasGoMod)
		v.HasGoMod = hasGoMod
		v.Packages = []*internal.Package{{Path: v.ModulePath + "/bar", Name: "bar"}}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	for _, hasGoMod := range []bool{true, false} {
		packagePath := fmt.Sprintf("foo.com/%t/bar", hasGoMod)
		sd, err := testDB.getSearchDocument(ctx, packagePath)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", packagePath, err)
		}
		if sd.hasGoMod != hasGoMod {
			t.Errorf("got hasGoMod=%t want %t", sd.hasGoMod, hasGoMod)
		}
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
		if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
			t.Fatalf("testDB.UpdateSearchDocumentsImportedByCount(ctx): %v", err)
		}
	}
	validateImportedByCountAndGetSearchDocument := func(path string, count int) *searchDocument {
		t.Helper()
		sd, err := testDB.getSearchDocument(ctx, path)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", path, err)
		}
		if sd.importedByCount > 0 && sd.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("importedByCountUpdatedAt for package %q should not be empty if count > 0", path)
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

	// Nothing imports C, so it has never been updated.
	if !sdC.importedByCountUpdatedAt.IsZero() {
		t.Fatalf("pkgC imported_by_count_updated_at should be zero, but is %v", sdC.importedByCountUpdatedAt)
	}
	if sdA.importedByCountUpdatedAt.IsZero() {
		t.Fatal("pkgA imported_by_count_updated_at should be non-zero, but is zero")
	}

	// Test imported_by_count_updated_at for B has not changed.
	sdB = validateImportedByCountAndGetSearchDocument(pkgB.Path, 0)
	if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
		t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v", wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
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

func TestHllHash(t *testing.T) {
	tests := []string{
		"",
		"The lazy fox.",
		"golang.org/x/tools",
		"Hello, 世界",
	}
	for _, test := range tests {
		h := md5.New()
		io.WriteString(h, test)
		want := int64(binary.BigEndian.Uint64(h.Sum(nil)[0:8]))
		row := testDB.db.QueryRow(context.Background(), "SELECT hll_hash($1)", test)
		var got int64
		if err := row.Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("hll_hash(%q) = %d, want %d", test, got, want)
		}
	}
}

func TestHllZeros(t *testing.T) {
	tests := []struct {
		i    int64
		want int
	}{
		{-1, 0},
		{-(1 << 63), 0},
		{0, 64},
		{1, 63},
		{1 << 31, 32},
		{1 << 62, 1},
		{(1 << 63) - 1, 1},
	}
	for _, test := range tests {
		row := testDB.db.QueryRow(context.Background(), "SELECT hll_zeros($1)", test.i)
		var got int
		if err := row.Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("hll_zeros(%d) = %d, want %d", test.i, got, test.want)
		}
	}
}

func TestDeleteOlderVersionFromSearch(t *testing.T) {
	ctx := context.Background()
	defer ResetTestDB(testDB, t)

	const (
		modulePath = "deleteme.com"
		version    = "v1.2.3" // delete older than this
	)

	type module struct {
		path        string
		version     string
		pkg         string
		wantDeleted bool
	}
	insert := func(m module) {
		v := sample.Version()
		v.ModulePath = m.path
		v.Version = m.version
		v.Packages = []*internal.Package{{Path: m.path + "/" + m.pkg, Name: m.pkg}}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	check := func(m module) {
		gotPath, gotVersion, found := GetFromSearchDocuments(ctx, t, testDB, m.path+"/"+m.pkg)
		gotDeleted := !found
		if gotDeleted != m.wantDeleted {
			t.Errorf("%s: gotDeleted=%t, wantDeleted=%t", m.path, gotDeleted, m.wantDeleted)
			return
		}
		if !gotDeleted {
			if gotPath != m.path || gotVersion != m.version {
				t.Errorf("got path, version (%q, %q), want (%q, %q)", gotPath, gotVersion, m.path, m.version)
			}
		}
	}

	modules := []module{
		{modulePath, "v1.1.0", "p2", true},   // older version of same module
		{modulePath, "v0.0.9", "p3", true},   // another older version of same module
		{"other.org", "v1.1.2", "p4", false}, // older version of a different module
	}
	for _, m := range modules {
		insert(m)
	}
	if err := testDB.DeleteOlderVersionFromSearchDocuments(ctx, modulePath, version); err != nil {
		t.Fatal(err)
	}

	for _, m := range modules {
		check(m)
	}

	// Verify that a newer version is not deleted.
	ResetTestDB(testDB, t)
	mod := module{modulePath, "v1.2.4", "p5", false}
	insert(mod)
	check(mod)
}
