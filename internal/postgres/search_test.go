// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/lib/pq"
	"go.opencensus.io/stats/view"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"
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
				"client",
				"client-go",
				"go",
				"k8s",
				"k8s.io",
				"k8s.io/client-go",
			},
		},
		{
			path: "github.com/foo/bar",
			want: []string{
				"bar",
				"foo",
				"foo/bar",
				"github.com/foo",
				"github.com/foo/bar",
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
				"code.cloud.gitlab.google.k8s.io",
				"k8s",
			},
		},
		{
			path: "/",
			want: nil,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := GeneratePathTokens(tc.path)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("generatePathTokens(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

// importGraph constructs a simple import graph where all importers import
// one popular package.  For performance purposes, all importers are added to
// a single importing module.
func importGraph(popularPath, importerModule string, importerCount int) []*internal.Module {
	m := sample.Module(popularPath, "v1.2.3", "")
	m.LegacyPackages[0].Imports = nil
	// Try to improve the ts_rank of the 'foo' search term.
	m.LegacyPackages[0].Synopsis = "foo"
	m.LegacyReadmeContents = "foo"
	mods := []*internal.Module{m}
	if importerCount > 0 {
		m := sample.Module(importerModule, "v1.2.3")
		for i := 0; i < importerCount; i++ {
			p := sample.LegacyPackage(importerModule, fmt.Sprintf("importer%d", i))
			p.Imports = []string{popularPath}
			sample.AddPackage(m, p)
		}
		mods = append(mods, m)
	}
	return mods
}

// resultGuard returns a 'guard' func for the search result ordering specified
// by resultOrder. When called with a searcher name, this func blocks until
// that result may be processed, then returns a callback to release the next
// result in the series.
//
// This is used to control the order of search results in hedgedSearch.
func resultGuard(resultOrder []string) func(string) func() {
	done := make(map[string](chan struct{}))
	// waitFor maps [search type] -> [the search type is should wait for]
	waitFor := make(map[string]string)
	for i := 0; i < len(resultOrder); i++ {
		done[resultOrder[i]] = make(chan struct{})
		if i > 0 {
			waitFor[resultOrder[i]] = resultOrder[i-1]
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
	return guardTestResult
}

func TestSearch(t *testing.T) {
	tests := []struct {
		label       string
		modules     []*internal.Module
		resultOrder []string
		wantSource  string
		wantResults []string
		wantTotal   uint64
	}{
		{
			label:       "single package",
			modules:     importGraph("foo.com/A", "", 0),
			resultOrder: []string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/A"},
			wantTotal:   1,
		},
		{
			label:       "empty results",
			modules:     []*internal.Module{},
			resultOrder: []string{"deep", "estimate", "popular"},
			wantSource:  "deep",
			wantResults: nil,
		},
		{
			label:       "both popular and unpopular results",
			modules:     importGraph("foo.com/popular", "bar.com/foo", 10),
			resultOrder: []string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popular", "bar.com/foo/importer0"},
			wantTotal:   11, // HLL result count (happens to be right in this case)
		},
		{
			label: "popular results, estimate before deep",
			modules: append(importGraph("foo.com/popularA", "bar.com", 60),
				importGraph("foo.com/popularB", "baz.com/foo", 70)...),
			resultOrder: []string{"popular", "estimate", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popularB", "foo.com/popularA"},
			wantTotal:   76, // HLL result count (actual count is 72)
		},
		{
			label: "popular results, deep before estimate",
			modules: append(importGraph("foo.com/popularA", "bar.com/foo", 60),
				importGraph("foo.com/popularB", "bar.com/foo", 70)...),
			resultOrder: []string{"popular", "deep", "estimate"},
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
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, v := range test.modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
				t.Fatal(err)
			}
			guardTestResult := resultGuard(test.resultOrder)
			resp, err := testDB.hedgedSearch(ctx, "foo", 2, 0, searchers, guardTestResult)
			if err != nil {
				t.Fatal(err)
			}
			if resp.source != test.wantSource {
				t.Errorf("hedgedSearch(): got source %q, want %q", resp.source, test.wantSource)
			}
			var got []string
			for _, r := range resp.results {
				got = append(got, r.PackagePath)
			}
			if diff := cmp.Diff(test.wantResults, got); diff != "" {
				t.Errorf("hedgedSearch() mismatch (-want +got)\n%s", diff)
			}
			if len(resp.results) > 0 && resp.results[0].NumResults != test.wantTotal {
				t.Errorf("NumResults = %d, want %d", resp.results[0].NumResults, test.wantTotal)
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

func TestSearchErrors(t *testing.T) {
	// errorIn returns a copy of searchers for which searcherName returns an
	// error.
	errorIn := func(searcherName string) map[string]searcher {
		newSearchers := make(map[string]searcher)
		for name, search := range searchers {
			if name == searcherName {
				name := name
				newSearchers[name] = func(*DB, context.Context, string, int, int) searchResponse {
					return searchResponse{
						source: name,
						err:    errors.New("bad"),
					}
				}
			} else {
				newSearchers[name] = search
			}
		}
		return newSearchers
	}

	tests := []struct {
		label       string
		searchers   map[string]searcher // allows injecting errors.
		resultOrder []string
		wantSource  string
		wantErr     bool
	}{
		{
			label:       "error in first result",
			searchers:   errorIn("popular"),
			resultOrder: []string{"popular", "estimate", "deep"},
			wantErr:     true,
		},
		{
			label:       "return before error",
			searchers:   errorIn("deep"),
			resultOrder: []string{"popular", "estimate", "deep"},
			wantSource:  "popular",
		},
		{
			label:       "error waiting for count",
			searchers:   errorIn("deep"),
			resultOrder: []string{"popular", "deep", "estimate"},
			wantErr:     true,
		},
		{
			label:       "counted result before error",
			searchers:   errorIn("popular"),
			resultOrder: []string{"deep", "popular", "estimate"},
			wantSource:  "deep",
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			modules := importGraph("foo.com/A", "", 0)
			for _, v := range modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
				t.Fatal(err)
			}
			guardTestResult := resultGuard(test.resultOrder)
			resp, err := testDB.hedgedSearch(ctx, "foo", 2, 0, test.searchers, guardTestResult)
			if (err != nil) != test.wantErr {
				t.Fatalf("hedgedSearch(): got error %v, want error: %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if resp.source != test.wantSource {
				t.Errorf("hedgedSearch(): got source %q, want %q", resp.source, test.wantSource)
			}
		})
	}
}

func TestInsertSearchDocumentAndSearch(t *testing.T) {
	var (
		modGoCDK = "gocloud.dev"
		pkgGoCDK = &internal.LegacyPackage{
			Name:              "cloud",
			Path:              "gocloud.dev/cloud",
			Synopsis:          "Package cloud contains a library and tools for open cloud development in Go. The Go Cloud Development Kit (Go CDK)",
			IsRedistributable: true, // required because some test cases depend on the README contents
		}

		modKube = "k8s.io"
		pkgKube = &internal.LegacyPackage{
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

	const (
		packageScore  = 0.6079270839691162
		goAndCDKScore = 0.999817967414856
		cloudScore    = 0.8654518127441406
	)

	for _, tc := range []struct {
		name          string
		packages      map[string]*internal.LegacyPackage
		limit, offset int
		searchQuery   string
		want          []*internal.SearchResult
	}{
		{
			name:        "two documents, single term search",
			searchQuery: "package",
			packages: map[string]*internal.LegacyPackage{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*internal.SearchResult{
				goCdkResult(packageScore, 2),
				kubeResult(packageScore, 2),
			},
		},
		{
			name:        "two documents, single term search, two results limit 1 offset 0",
			limit:       1,
			offset:      0,
			searchQuery: "package",
			packages: map[string]*internal.LegacyPackage{
				modKube:  pkgKube,
				modGoCDK: pkgGoCDK,
			},
			want: []*internal.SearchResult{
				goCdkResult(packageScore, 2),
			},
		},
		{
			name:        "two documents, single term search, two results limit 1 offset 1",
			limit:       1,
			offset:      1,
			searchQuery: "package",
			packages: map[string]*internal.LegacyPackage{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*internal.SearchResult{
				kubeResult(packageScore, 2),
			},
		},
		{
			name:        "two documents, multiple term search",
			searchQuery: "go & cdk",
			packages: map[string]*internal.LegacyPackage{
				modGoCDK: pkgGoCDK,
				modKube:  pkgKube,
			},
			want: []*internal.SearchResult{
				goCdkResult(goAndCDKScore, 1),
			},
		},
		{
			name:        "one document, single term search",
			searchQuery: "cloud",
			packages: map[string]*internal.LegacyPackage{
				modGoCDK: pkgGoCDK,
			},
			want: []*internal.SearchResult{
				goCdkResult(cloudScore, 1),
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
					m := sample.Module(modulePath, sample.VersionString)
					sample.AddPackage(m, pkg)
					if err := testDB.InsertModule(ctx, m); err != nil {
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

func TestSearchPenalties(t *testing.T) {
	// Verify that the penalties for non-redistributable modules and modules without
	// go.mod files are applied correctly.
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// All these modules will have the same text ranking for the search term "foo",
	// but different scores due to penalties.
	modules := map[string]struct {
		redist     bool
		hasGoMod   bool
		multiplier float64 // applied to base score
	}{
		"both.com/foo":      {true, true, 1},
		"nogomod.com/foo":   {true, false, noGoModPenalty},
		"nonredist.com/foo": {false, true, nonRedistributablePenalty},
		"neither.com/foo":   {false, false, noGoModPenalty * nonRedistributablePenalty},
	}

	for path, m := range modules {
		v := sample.Module(path, sample.VersionString, "p")
		v.LegacyPackages[0].IsRedistributable = m.redist
		v.IsRedistributable = m.redist
		v.HasGoMod = m.hasGoMod
		if err := testDB.InsertModule(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	for method, searcher := range searchers {
		t.Run(method, func(t *testing.T) {
			res := searcher(testDB, ctx, "foo", 10, 0)
			if res.err != nil {
				t.Fatal(res.err)
			}
			if got, want := len(res.results), len(modules); got != want {
				t.Fatalf("got %d search results, want %d", got, want)
			}
			for _, r := range res.results {
				got := r.Score
				want := res.results[0].Score * modules[r.ModulePath].multiplier
				if math.Abs(got-want) > 1e6 {
					t.Errorf("%s: got %f, want %f", r.ModulePath, got, want)
				}
			}
		})
	}
}

func TestExcludedFromSearch(t *testing.T) {
	// Verify that excluded paths are omitted from search results.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	// Insert a module with two packages.
	const domain = "exclude.com"
	sm := sample.Module(domain, "v1.2.3", "pkg", "exclude")
	if err := testDB.InsertModule(ctx, sm); err != nil {
		t.Fatal(err)
	}
	// Exclude a prefix that matches one of the packages.
	if err := testDB.InsertExcludedPrefix(ctx, domain+"/ex", "no user", "no reason"); err != nil {
		t.Fatal(err)
	}
	// Search for both packages.
	gotResults, err := testDB.Search(ctx, domain, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range gotResults {
		got = append(got, g.Name)
	}
	want := []string{"pkg"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSearchBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	m := nonRedistributableModule()
	if err := bypassDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		db        *DB
		wantEmpty bool
	}{
		{testDB, true},
		{bypassDB, false},
	} {
		rs, err := test.db.Search(ctx, m.ModulePath, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := (rs[0].Synopsis == ""); got != test.wantEmpty {
			t.Errorf("bypass %t: got empty %t, want %t", test.db == bypassDB, got, test.wantEmpty)
		}
	}
}

type searchDocument struct {
	packagePath              string
	modulePath               string
	version                  string
	commitTime               time.Time
	name                     string
	synopsis                 string
	licenseTypes             []string
	importedByCount          int
	redistributable          bool
	hasGoMod                 bool
	versionUpdatedAt         time.Time
	importedByCountUpdatedAt time.Time
}

// getSearchDocument returns the search_document for the package with the given
// path. It is only used for testing purposes.
func getSearchDocument(ctx context.Context, db *DB, path string) (*searchDocument, error) {
	query := `
		SELECT
			package_path,
			module_path,
			version,
			commit_time,
			name,
			synopsis,
			license_types,
			imported_by_count,
			redistributable,
			has_go_mod,
			version_updated_at,
			imported_by_count_updated_at
		FROM
			search_documents
		WHERE package_path=$1`
	row := db.db.QueryRow(ctx, query, path)
	var (
		sd searchDocument
		t  pq.NullTime
	)
	if err := row.Scan(&sd.packagePath, &sd.modulePath, &sd.version, &sd.commitTime,
		&sd.name, &sd.synopsis, pq.Array(&sd.licenseTypes), &sd.importedByCount,
		&sd.redistributable, &sd.hasGoMod, &sd.versionUpdatedAt, &t); err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	if t.Valid {
		sd.importedByCountUpdatedAt = t.Time
	}
	return &sd, nil
}

func TestUpsertSearchDocument(t *testing.T) {
	// Verify that inserting into search_documents populates all columns correctly,
	// both with and without a conflict.
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var packagePath = sample.ModulePath + "/A"

	getSearchDocument := func() *searchDocument {
		t.Helper()
		sd, err := getSearchDocument(ctx, testDB, packagePath)
		if err != nil {
			t.Fatal(err)
		}
		return sd
	}

	insertModule := func(version string, gomod bool) {
		v := sample.Module(sample.ModulePath, version, "A")
		v.HasGoMod = gomod
		v.LegacyPackages[0].Synopsis = "syn-" + version
		if err := testDB.InsertModule(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	insertModule("v1.0.0", false)

	// Ensures the row updated in the search_document table for a given module
	// is updated with a version that prefers non-incompatible modules. (The
	// latest version after this insertion should remain v1.0.0)
	insertModule("v2.0.0+incompatible", false)
	sdOriginal := getSearchDocument()

	insertModule("v0.5.0", true)
	sdOlder := getSearchDocument()
	if diff := cmp.Diff(sdOriginal, sdOlder, cmp.AllowUnexported(searchDocument{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}

	insertModule("v1.5.2", true)
	sdNewer := getSearchDocument()
	if orig, newer := sdOriginal.versionUpdatedAt, sdNewer.versionUpdatedAt; orig == newer {
		t.Fatalf("expected version_updated_at to change since a newer version was inserted; got original = %v, newer = %v",
			orig, newer)
	}
	sdWant := sdOriginal
	sdWant.version = "v1.5.2"
	sdWant.synopsis = "syn-v1.5.2"
	sdWant.hasGoMod = true
	sdWant.versionUpdatedAt = sdNewer.versionUpdatedAt
	if diff := cmp.Diff(sdWant, sdNewer, cmp.AllowUnexported(searchDocument{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestUpsertSearchDocumentVersionHasGoMod(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, hasGoMod := range []bool{true, false} {
		m := sample.Module(fmt.Sprintf("foo.com/%t", hasGoMod), "v1.2.3", "bar")
		m.HasGoMod = hasGoMod
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, hasGoMod := range []bool{true, false} {
		packagePath := fmt.Sprintf("foo.com/%t/bar", hasGoMod)
		sd, err := getSearchDocument(ctx, testDB, packagePath)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", packagePath, err)
		}
		if sd.hasGoMod != hasGoMod {
			t.Errorf("got hasGoMod=%t want %t", sd.hasGoMod, hasGoMod)
		}
	}
}

func TestUpdateSearchDocumentsImportedByCount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// insert package with suffix at version, return the module
	insertPackageVersion := func(suffix, version string, imports []string) *internal.Module {
		t.Helper()
		m := sample.Module("mod.com/"+suffix, version, suffix)
		pkg := m.LegacyPackages[0]
		pkg.Imports = nil
		for _, imp := range imports {
			pkg.Imports = append(pkg.Imports, fmt.Sprintf("mod.com/%s/%[1]s", imp))
		}
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	updateImportedByCount := func() {
		t.Helper()
		if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
			t.Fatal(err)
		}
	}
	validateImportedByCountAndGetSearchDocument := func(path string, count int) *searchDocument {
		t.Helper()
		sd, err := getSearchDocument(ctx, testDB, path)
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

	pkgPath := func(m *internal.Module) string { return m.LegacyPackages[0].Path }

	t.Run("basic", func(t *testing.T) {
		defer ResetTestDB(testDB, t)

		// Test imported_by_count = 0 when only pkgA is added.
		mA := insertPackageVersion("A", "v1.0.0", nil)
		updateImportedByCount()
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mA), 0)

		// Test imported_by_count = 1 for pkgA when pkgB is added.
		mB := insertPackageVersion("B", "v1.0.0", []string{"A"})
		updateImportedByCount()
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mA), 1)
		sdB := validateImportedByCountAndGetSearchDocument(pkgPath(mB), 0)
		wantSearchDocBUpdatedAt := sdB.importedByCountUpdatedAt

		// Test imported_by_count = 2 for pkgA, when C is added.
		mC := insertPackageVersion("C", "v1.0.0", []string{"A"})
		updateImportedByCount()
		sdA := validateImportedByCountAndGetSearchDocument(pkgPath(mA), 2)
		sdC := validateImportedByCountAndGetSearchDocument(pkgPath(mC), 0)

		// Nothing imports C, so it has never been updated.
		if !sdC.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("pkgC imported_by_count_updated_at should be zero, but is %v", sdC.importedByCountUpdatedAt)
		}
		if sdA.importedByCountUpdatedAt.IsZero() {
			t.Fatal("pkgA imported_by_count_updated_at should be non-zero, but is zero")
		}

		// Test imported_by_count_updated_at for B has not changed.
		sdB = validateImportedByCountAndGetSearchDocument(pkgPath(mB), 0)
		if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
			t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v",
				wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
		}

		// When an older version of A imports D, nothing happens to the counts,
		// because imports_unique only records the latest version of each package.
		mD := insertPackageVersion("D", "v1.0.0", nil)
		insertPackageVersion("A", "v0.9.0", []string{"D"})
		updateImportedByCount()
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mD), 0)

		// When a newer version of A imports D, however, the counts do change.
		insertPackageVersion("A", "v1.1.0", []string{"D"})
		updateImportedByCount()
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(pkgPath(mD), 1)
	})

	t.Run("alternative", func(t *testing.T) {
		// Test with alternative modules that are removed from search_documents.
		defer ResetTestDB(testDB, t)

		insertPackageVersion("B", "v1.0.0", nil)

		// Insert a package with the canonical module path.
		canonicalModule := insertPackageVersion("A", "v1.0.0", []string{"B"})

		// Imagine we see a package with an alternative path at v1.2.0.
		// We add that information to module_version_states.
		alternativeModulePath := strings.ToLower(canonicalModule.ModulePath)
		alternativeStatus := derrors.ToStatus(derrors.AlternativeModule)
		err := testDB.UpsertModuleVersionState(ctx, alternativeModulePath, "v1.2.0", "",
			time.Now(), alternativeStatus, canonicalModule.ModulePath, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Now we see an earlier version of that package, without a go.mod file, so we insert it.
		// It should not get inserted into search_documents.
		mAlt := sample.Module(alternativeModulePath, "v1.0.0", "A")
		mAlt.LegacyPackages[0].Imports = []string{"B"}
		if err := testDB.InsertModule(ctx, mAlt); err != nil {
			t.Fatal(err)
		}
		// Although B is imported by two packages, only one is in search_documents, so its
		// imported-by count is 1.
		updateImportedByCount()
		validateImportedByCountAndGetSearchDocument("mod.com/B/B", 1)
	})
}

func TestGetPackagesForSearchDocumentUpsert(t *testing.T) {
	defer ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	moduleA := sample.Module("mod.com", "v1.2.3",
		"A", "A/notinternal", "A/internal", "A/internal/B")
	if err := testDB.InsertModule(ctx, moduleA); err != nil {
		t.Fatal(err)
	}

	// We are asking for all packages in search_documents updated before now, which is
	// all the non-internal packages.
	got, err := testDB.GetPackagesForSearchDocumentUpsert(ctx, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].PackagePath < got[j].PackagePath })
	want := []upsertSearchDocumentArgs{
		{
			PackagePath:    "mod.com/A",
			ModulePath:     "mod.com",
			ReadmeFilePath: "README.md",
			ReadmeContents: "readme",
			Synopsis:       "This is a package synopsis",
		},
		{
			PackagePath:    "mod.com/A/notinternal",
			ModulePath:     "mod.com",
			ReadmeFilePath: "README.md",
			ReadmeContents: "readme",
			Synopsis:       "This is a package synopsis",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("testDB.GetPackagesForSearchDocumentUpsert mismatch(-want +got):\n%s", diff)
	}

	// pkgPaths should be an empty slice, all packages were inserted more recently than yesterday.
	got, err = testDB.GetPackagesForSearchDocumentUpsert(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected testDB.GetPackagesForSearchDocumentUpsert to return an empty slice; got %v", got)
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
		sm := sample.Module(m.path, m.version, m.pkg)
		if err := testDB.InsertModule(ctx, sm); err != nil {
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
