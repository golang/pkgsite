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
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestPathTokens(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
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
		t.Run(test.path, func(t *testing.T) {
			got := GeneratePathTokens(test.path)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("generatePathTokens(%q) mismatch (-want +got):\n%s", test.path, diff)
			}
		})
	}
}

// importGraph constructs a simple import graph where all importers import
// one popular package.  For performance purposes, all importers are added to
// a single importing module.
func importGraph(popularPath, importerModule string, importerCount int) []*internal.Module {
	m := sample.Module(popularPath, "v1.2.3", "")
	m.Packages()[0].Imports = nil
	// Try to improve the ts_rank of the 'foo' search term.
	m.Packages()[0].Documentation[0].Synopsis = "foo"
	m.Units[0].Readme.Contents = "foo"
	mods := []*internal.Module{m}

	if importerCount > 0 {
		m := sample.Module(importerModule, "v1.2.3")
		for i := 0; i < importerCount; i++ {
			name := fmt.Sprintf("importer%d", i)
			fullPath := importerModule + "/" + name
			u := &internal.Unit{
				UnitMeta:      *sample.UnitMeta(fullPath, importerModule, m.Version, name, true),
				Documentation: []*internal.Documentation{sample.Doc},
				Imports:       []string{popularPath},
			}
			sample.AddUnit(m, u)
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
		if await, ok := waitFor[source]; ok {
			<-done[await]
		}
		return func() { close(done[source]) }
	}
	return guardTestResult
}

func TestSearch(t *testing.T) {
	// Cannot be run in parallel with other search tests, because it reads
	// metrics before and after (see responseDelta below).
	ctx := context.Background()
	tests := []struct {
		label       string
		modules     []*internal.Module
		resultOrder []string
		wantSource  string
		wantResults []string
		wantTotal   uint64
	}{
		{
			label:       "single package from popular",
			modules:     importGraph("foo.com/A", "", 0),
			resultOrder: []string{"popular", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/A"},
			wantTotal:   1,
		},
		{
			label:       "single package from deep",
			modules:     importGraph("foo.com/A", "", 0),
			resultOrder: []string{"deep", "popular"},
			wantSource:  "deep",
			wantResults: []string{"foo.com/A"},
			wantTotal:   1,
		},
		{
			label:       "empty results",
			modules:     []*internal.Module{},
			resultOrder: []string{"deep", "popular"},
			wantSource:  "deep",
			wantResults: nil,
		},
		{
			label:       "both popular and unpopular results",
			modules:     importGraph("foo.com/popular", "bar.com/foo", 10),
			resultOrder: []string{"popular", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popular", "bar.com/foo/importer0"},
			wantTotal:   100, // popular assumes 100 results
		},
		{
			label: "popular before deep",
			modules: append(importGraph("foo.com/popularA", "bar.com", 60),
				importGraph("foo.com/popularB", "baz.com/foo", 70)...),
			resultOrder: []string{"popular", "deep"},
			wantSource:  "popular",
			wantResults: []string{"foo.com/popularB", "foo.com/popularA"},
			wantTotal:   100, // popular assumes 100 results
		},
		{
			label: "deep before popular",
			modules: append(importGraph("foo.com/popularA", "bar.com/foo", 60),
				importGraph("foo.com/popularB", "bar.com/foo", 70)...),
			resultOrder: []string{"deep", "popular"},
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
			testDB, release := acquire(t)
			defer release()
			for _, m := range test.modules {
				MustInsertModule(ctx, t, testDB, m)
			}
			if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
				t.Fatal(err)
			}
			guardTestResult := resultGuard(test.resultOrder)
			resp, err := testDB.hedgedSearch(ctx, "foo", 2, 0, 100, searchers, guardTestResult)
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
	t.Parallel()
	ctx := context.Background()
	// errorIn returns a copy of searchers for which searcherName returns an
	// error.
	errorIn := func(searcherName string) map[string]searcher {
		newSearchers := make(map[string]searcher)
		for name, search := range searchers {
			if name == searcherName {
				name := name
				newSearchers[name] = func(*DB, context.Context, string, int, int, int) searchResponse {
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
			resultOrder: []string{"popular", "deep"},
			wantErr:     true,
		},
		{
			label:       "return before error",
			searchers:   errorIn("deep"),
			resultOrder: []string{"popular", "deep"},
			wantSource:  "popular",
		},
		{
			label:       "counted result before error",
			searchers:   errorIn("popular"),
			resultOrder: []string{"deep", "popular"},
			wantSource:  "deep",
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()
			modules := importGraph("foo.com/A", "", 0)
			for _, v := range modules {
				MustInsertModule(ctx, t, testDB, v)
			}
			if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
				t.Fatal(err)
			}
			guardTestResult := resultGuard(test.resultOrder)
			resp, err := testDB.hedgedSearch(ctx, "foo", 2, 0, 100, test.searchers, guardTestResult)
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
	t.Parallel()
	ctx := context.Background()
	var (
		modGoCDK = "gocloud.dev"
		pkgGoCDK = &internal.Unit{
			UnitMeta: internal.UnitMeta{
				Name:              "cloud",
				Path:              "gocloud.dev/cloud",
				IsRedistributable: true, // required because some test cases depend on the README contents
			},
			Documentation: []*internal.Documentation{{
				GOOS:     sample.GOOS,
				GOARCH:   sample.GOARCH,
				Synopsis: "Package cloud contains a library and tools for open cloud development in Go. The Go Cloud Development Kit (Go CDK)",
				Source:   []byte{},
			}},
		}

		modKube = "k8s.io"
		pkgKube = &internal.Unit{
			UnitMeta: internal.UnitMeta{
				Name:              "client-go",
				Path:              "k8s.io/client-go",
				IsRedistributable: true, // required because some test cases depend on the README contents
			},
			Documentation: []*internal.Documentation{{
				GOOS:     sample.GOOS,
				GOARCH:   sample.GOARCH,
				Synopsis: "Package client-go implements a Go client for Kubernetes.",
				Source:   []byte{},
			}},
		}

		kubeResult = func(score float64, numResults uint64) *internal.SearchResult {
			return &internal.SearchResult{
				Name:        pkgKube.Name,
				PackagePath: pkgKube.Path,
				Synopsis:    pkgKube.Documentation[0].Synopsis,
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
				Synopsis:    pkgGoCDK.Documentation[0].Synopsis,
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

	for _, test := range []struct {
		name          string
		packages      map[string]*internal.Unit
		limit, offset int
		searchQuery   string
		want          []*internal.SearchResult
	}{
		{
			name:        "two documents, single term search",
			searchQuery: "package",
			packages: map[string]*internal.Unit{
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
			packages: map[string]*internal.Unit{
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
			packages: map[string]*internal.Unit{
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
			packages: map[string]*internal.Unit{
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
			packages: map[string]*internal.Unit{
				modGoCDK: pkgGoCDK,
			},
			want: []*internal.SearchResult{
				goCdkResult(cloudScore, 1),
			},
		},
	} {
		for method, searcher := range searchers {
			t.Run(test.name+":"+method, func(t *testing.T) {
				testDB, release := acquire(t)
				defer release()

				for modulePath, pkg := range test.packages {
					pkg.Licenses = sample.LicenseMetadata()
					m := sample.Module(modulePath, sample.VersionString)
					sample.AddUnit(m, pkg)
					MustInsertModule(ctx, t, testDB, m)
				}

				if test.limit < 1 {
					test.limit = 10
				}

				got := searcher(testDB, ctx, test.searchQuery, test.limit, test.offset, 100)
				if got.err != nil {
					t.Fatal(got.err)
				}
				// Normally done by hedgedSearch, but we're bypassing that.
				if err := testDB.addPackageDataToSearchResults(ctx, got.results); err != nil {
					t.Fatal(err)
				}
				if len(got.results) != len(test.want) {
					t.Errorf("testDB.Search(%v, %d, %d) mismatch: len(got) = %d, want = %d\n", test.searchQuery, test.limit, test.offset, len(got.results), len(test.want))
				}

				// The searchers differ in these two fields.
				opt := cmpopts.IgnoreFields(internal.SearchResult{}, "Approximate", "NumResults")
				if diff := cmp.Diff(test.want, got.results, opt); diff != "" {
					t.Errorf("testDB.Search(%v, %d, %d) mismatch (-want +got):\n%s", test.searchQuery, test.limit, test.offset, diff)
				}
			})
		}
	}
}

func TestSearchPenalties(t *testing.T) {
	// Verify that the penalties for non-redistributable modules and modules without
	// go.mod files are applied correctly.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
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
		v.Packages()[0].IsRedistributable = m.redist
		v.IsRedistributable = m.redist
		v.HasGoMod = m.hasGoMod
		MustInsertModule(ctx, t, testDB, v)
	}

	for method, searcher := range searchers {
		t.Run(method, func(t *testing.T) {
			res := searcher(testDB, ctx, "foo", 10, 0, 100)
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Insert a module with two packages.
	const domain = "exclude.com"
	sm := sample.Module(domain, "v1.2.3", "pkg", "ex/clude")
	MustInsertModule(ctx, t, testDB, sm)
	// Exclude a prefix that matches one of the packages.
	if err := testDB.InsertExcludedPrefix(ctx, domain+"/ex", "no user", "no reason"); err != nil {
		t.Fatal(err)
	}
	// Search for both packages.
	gotResults, err := testDB.Search(ctx, domain, 10, 0, 100)
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	m := nonRedistributableModule()
	MustInsertModule(ctx, t, bypassDB, m)

	for _, test := range []struct {
		db        *DB
		wantEmpty bool
	}{
		{testDB, true},
		{bypassDB, false},
	} {
		rs, err := test.db.Search(ctx, m.ModulePath, 10, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if got := (rs[0].Synopsis == ""); got != test.wantEmpty {
			t.Errorf("bypass %t: got empty %t, want %t", test.db == bypassDB, got, test.wantEmpty)
		}
	}
}

func TestSearchLicenseDedup(t *testing.T) {
	// Verify that a license type appears only once even if there are multiple
	// licenses of that type.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()
	m := sample.Module("example.com/mod", "v1.2.3", "pkg")
	// Add a second MIT license in the pkg directory.
	sample.AddLicense(m, &licenses.License{
		Metadata: &licenses.Metadata{
			Types:    []string{"MIT"},
			FilePath: "pkg/LICENSE.md",
		},
	})
	MustInsertModule(ctx, t, testDB, m)
	got, err := testDB.Search(ctx, m.ModulePath, 10, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got, want := got[0].Licenses, []string{"MIT"}; !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
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
		m := sample.Module(sample.ModulePath, version, "A")
		m.HasGoMod = gomod
		m.Packages()[0].Documentation[0].Synopsis = "syn-" + version
		MustInsertModule(ctx, t, testDB, m)
	}

	const v1 = "v1.0.0"
	insertModule(v1, false)
	sd1 := getSearchDocument()
	if sd1.version != v1 {
		t.Fatalf("got %s, want %s", sd1.version, v1)
	}

	// Since the latest compatible version has no go.mod file, this incompatible version
	// is the latest.
	const vInc = "v2.0.0+incompatible"
	insertModule(vInc, false)
	sdInc := getSearchDocument()
	if sdInc.version != vInc {
		t.Fatalf("got %s, want %s", sdInc.version, vInc)
	}

	// Inserting an older module doesn't change anything.
	insertModule("v0.5.0", true)
	sdOlder := getSearchDocument()
	if diff := cmp.Diff(sdInc, sdOlder, cmp.AllowUnexported(searchDocument{})); diff != "" {
		t.Fatalf("mismatch (-want, +got):\n%s", diff)
	}

	// A later compatible version with a go.mod file. This becomes the new latest version.
	const v15 = "v1.5.2"
	insertModule(v15, true)
	sdNewer := getSearchDocument()
	if sdNewer.version != v15 {
		t.Fatalf("got %s, want %s", sdNewer.version, v15)
	}
	sdWant := sd1
	sdWant.version = "v1.5.2"
	sdWant.synopsis = "syn-v1.5.2"
	sdWant.hasGoMod = true
	sdWant.versionUpdatedAt = sdNewer.versionUpdatedAt
	if diff := cmp.Diff(sdWant, sdNewer, cmp.AllowUnexported(searchDocument{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestUpsertSearchDocumentVersionHasGoMod(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, hasGoMod := range []bool{true, false} {
		m := sample.Module(fmt.Sprintf("foo.com/%t", hasGoMod), "v1.2.3", "bar")
		m.HasGoMod = hasGoMod
		MustInsertModule(ctx, t, testDB, m)
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
	t.Parallel()
	ctx := context.Background()

	pkgPath := func(m *internal.Module) string { return m.Packages()[0].Path }

	// insert package with suffix at version, return the module
	insertPackageVersion := func(t *testing.T, db *DB, suffix, version string, imports []string) *internal.Module {
		t.Helper()
		m := sample.Module("mod.com/"+suffix, version, suffix)
		// Units[0] is the module itself.
		pkg := m.Units[1]
		pkg.Imports = nil
		for _, imp := range imports {
			pkg.Imports = append(pkg.Imports, fmt.Sprintf("mod.com/%s/%[1]s", imp))
		}
		MustInsertModule(ctx, t, db, m)
		return m
	}

	updateImportedByCount := func(db *DB) {
		t.Helper()
		if _, err := db.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
			t.Fatal(err)
		}
	}

	validateImportedByCountAndGetSearchDocument := func(t *testing.T, db *DB, path string, count int) *searchDocument {
		t.Helper()
		sd, err := getSearchDocument(ctx, db, path)
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

	t.Run("basic", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Test imported_by_count = 0 when only pkgA is added.
		mA := insertPackageVersion(t, testDB, "A", "v1.0.0", nil)
		updateImportedByCount(testDB)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 0)

		// Test imported_by_count = 1 for pkgA when pkgB is added.
		mB := insertPackageVersion(t, testDB, "B", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 1)
		sdB := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0)
		wantSearchDocBUpdatedAt := sdB.importedByCountUpdatedAt

		// Test imported_by_count = 2 for pkgA, when C is added.
		mC := insertPackageVersion(t, testDB, "C", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB)
		sdA := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		sdC := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mC), 0)

		// Nothing imports C, so it has never been updated.
		if !sdC.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("pkgC imported_by_count_updated_at should be zero, but is %v", sdC.importedByCountUpdatedAt)
		}
		if sdA.importedByCountUpdatedAt.IsZero() {
			t.Fatal("pkgA imported_by_count_updated_at should be non-zero, but is zero")
		}

		// Test imported_by_count_updated_at for B has not changed.
		sdB = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0)
		if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
			t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v",
				wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
		}

		// When an older version of A imports D, nothing happens to the counts,
		// because imports_unique only records the latest version of each package.
		mD := insertPackageVersion(t, testDB, "D", "v1.0.0", nil)
		insertPackageVersion(t, testDB, "A", "v0.9.0", []string{"D"})
		updateImportedByCount(testDB)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 0)

		// When a newer version of A imports D, however, the counts do change.
		insertPackageVersion(t, testDB, "A", "v1.1.0", []string{"D"})
		updateImportedByCount(testDB)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 1)
	})

	t.Run("alternative", func(t *testing.T) {
		// Test with alternative modules that are removed from search_documents.
		testDB, release := acquire(t)
		defer release()

		insertPackageVersion(t, testDB, "B", "v1.0.0", nil)

		// Insert a package with the canonical module path.
		canonicalModule := insertPackageVersion(t, testDB, "A", "v1.0.0", []string{"B"})

		// Imagine we see a package with an alternative path at v1.2.0.
		// We add that information to module_version_states.
		alternativeModulePath := strings.ToLower(canonicalModule.ModulePath)
		alternativeStatus := derrors.ToStatus(derrors.AlternativeModule)
		mvs := &ModuleVersionStateForUpsert{
			ModulePath: alternativeModulePath,
			Version:    "v1.2.0",
			Timestamp:  time.Now(),
			Status:     alternativeStatus,
			GoModPath:  canonicalModule.ModulePath,
		}
		if err := testDB.UpsertModuleVersionState(ctx, mvs); err != nil {
			t.Fatal(err)
		}

		// Now we see an earlier version of that package, without a go.mod file, so we insert it.
		// It should not get inserted into search_documents.
		mAlt := sample.Module(alternativeModulePath, "v1.0.0", "A")
		mAlt.Packages()[0].Imports = []string{"B"}
		MustInsertModule(ctx, t, testDB, mAlt)
		// Although B is imported by two packages, only one is in search_documents, so its
		// imported-by count is 1.
		updateImportedByCount(testDB)
		validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/B/B", 1)
	})
}

func TestGetPackagesForSearchDocumentUpsert(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	moduleA := sample.Module("mod.com", "v1.2.3",
		"A", "A/notinternal", "A/internal", "A/internal/B")

	// moduleA.Units[1] is mod.com/A.
	moduleA.Units[1].Readme = &internal.Readme{
		Filepath: sample.ReadmeFilePath,
		Contents: sample.ReadmeContents,
	}
	// moduleA.Units[2] is mod.com/A/notinternal.
	moduleA.Units[2].Readme = &internal.Readme{
		Filepath: sample.ReadmeFilePath,
		Contents: sample.ReadmeContents,
	}
	moduleN := nonRedistributableModule()
	bypassDB := NewBypassingLicenseCheck(testDB.db)
	for _, m := range []*internal.Module{moduleA, moduleN} {
		MustInsertModule(ctx, t, bypassDB, m)
	}

	// We are asking for all packages in search_documents updated before now, which is
	// all the non-internal packages.
	got, err := testDB.GetPackagesForSearchDocumentUpsert(ctx, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].PackagePath < got[j].PackagePath })
	want := []UpsertSearchDocumentArgs{
		{
			PackagePath:    moduleN.ModulePath,
			ModulePath:     moduleN.ModulePath,
			Version:        "v1.2.3",
			ReadmeFilePath: "",
			ReadmeContents: "",
			Synopsis:       "",
		},
		{
			PackagePath:    "mod.com/A",
			ModulePath:     "mod.com",
			Version:        "v1.2.3",
			ReadmeFilePath: "README.md",
			ReadmeContents: "readme",
			Synopsis:       sample.Doc.Synopsis,
		},
		{
			PackagePath:    "mod.com/A/notinternal",
			ModulePath:     "mod.com",
			Version:        "v1.2.3",
			ReadmeFilePath: "README.md",
			ReadmeContents: "readme",
			Synopsis:       sample.Doc.Synopsis,
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("testDB.GetPackagesForSearchDocumentUpsert mismatch(-want +got):\n%s", diff)
	}

	// Reading with license bypass should return the non-redistributable fields.
	got, err = bypassDB.GetPackagesForSearchDocumentUpsert(ctx, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("len(got)==0")
	}
	sort.Slice(got, func(i, j int) bool { return got[i].PackagePath < got[j].PackagePath })
	gm := got[0]
	for _, got := range []string{gm.ReadmeFilePath, gm.ReadmeContents, gm.Synopsis} {
		if got == "" {
			t.Errorf("got empty field, want non-empty")
		}
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()

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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
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
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

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
		MustInsertModule(ctx, t, testDB, sm)
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

func TestGroupSearchResults(t *testing.T) {
	rs := []*internal.SearchResult{
		{PackagePath: "m.com/p", ModulePath: "m.com", Score: 10},
		{PackagePath: "m.com/p2", ModulePath: "m.com", Score: 8},
		{PackagePath: "m.com/v2/p", ModulePath: "m.com/v2", Score: 6},
		{PackagePath: "m.com/v2/p2", ModulePath: "m.com/v2", Score: 4},
	}
	got := groupSearchResults(rs)
	sp2 := &internal.SearchResult{
		PackagePath: "m.com/p2",
		ModulePath:  "m.com",
		Score:       8,
	}
	sp := &internal.SearchResult{
		PackagePath: "m.com/p",
		ModulePath:  "m.com",
		Score:       10,
		SameModule:  []*internal.SearchResult{sp2},
	}
	want := []*internal.SearchResult{
		{
			PackagePath: "m.com/v2/p",
			ModulePath:  "m.com/v2",
			Score:       6,
			SameModule: []*internal.SearchResult{
				{PackagePath: "m.com/v2/p2", ModulePath: "m.com/v2", Score: 4},
			},
			LowerMajor: []*internal.SearchResult{sp, sp2},
		},
		sp,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got)\n%s", diff)
	}
}
