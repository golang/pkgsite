// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/vuln"
)

type fallbackDataSource struct {
	internal.DataSource
	fallbackMap map[string]string // requested module -> resolved module
}

func (ds fallbackDataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (*internal.UnitMeta, error) {
	if resolved, ok := ds.fallbackMap[requestedModulePath]; ok {
		um, err := ds.DataSource.GetUnitMeta(ctx, path, resolved, requestedVersion)
		if err != nil {
			return nil, err
		}
		return um, nil
	}
	return ds.DataSource.GetUnitMeta(ctx, path, requestedModulePath, requestedVersion)
}

// setupTestDB sets up a DB for testing.
// It also removes the voluminous DB log output,
// which makes it hard to see test information.
// Note: This function modifies global log state and should not
// be used in tests running with t.Parallel().
func setupTestDB(t *testing.T) internal.TestingDataSource {
	orig := log.GetLevel()
	t.Cleanup(func() { log.SetLevel(orig.String()) })
	log.SetLevel("Info")
	const testDB = "pkgsite_api"
	db, err := postgres.SetupTestDB(testDB)
	if err != nil {
		if errors.Is(err, derrors.NotFound) && os.Getenv("GO_DISCOVERY_TESTDB") != "true" {
			t.Skipf("could not connect to DB (see doc/postgres.md to set up): %v", err)
		}
		t.Fatalf("setting up DB: %v", err)
	}
	t.Cleanup(func() {
		postgres.ResetTestDB(db, t)
		db.Close()
	})
	return db
}

func TestAPI(t *testing.T) {
	t.Run("fake", func(t *testing.T) {
		testAPI(t, func(t *testing.T) internal.TestingDataSource {
			return fakedatasource.New()
		})
	})
	t.Run("db", func(t *testing.T) {
		testAPI(t, setupTestDB)
	})
}

func testAPI(t *testing.T, newTestingDataSource func(t *testing.T) internal.TestingDataSource) {
	t.Run("package", func(t *testing.T) {
		testServePackage(t, newTestingDataSource(t))
	})
	t.Run("module", func(t *testing.T) {
		testServeModule(t, newTestingDataSource(t))
	})
	t.Run("module versions", func(t *testing.T) {
		testServeModuleVersions(t, newTestingDataSource(t))
	})
	t.Run("module packages", func(t *testing.T) {
		testServeModulePackages(t, newTestingDataSource(t))
	})
	t.Run("search", func(t *testing.T) {
		testServeSearch(t, newTestingDataSource(t))
	})
	t.Run("search pagination", func(t *testing.T) {
		testServeSearchPagination(t, newTestingDataSource(t))
	})
	t.Run("package symbols", func(t *testing.T) {
		testServePackageSymbols(t, newTestingDataSource(t))
	})
	t.Run("package imported by", func(t *testing.T) {
		testServePackageImportedBy(t, newTestingDataSource(t))
	})
}

func testServePackage(t *testing.T, ds internal.TestingDataSource) {
	const (
		pkgPath     = "example.com/a/b"
		modulePath1 = "example.com/a"
		modulePath2 = "example.com/a/b"
		version     = "v1.2.3"
	)

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       version,
			LatestVersion: "v1.2.4",
		},
		Licenses: sample.Licenses(),
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "example.com",
					Version:           version,
					LatestVersion:     "v1.2.4",
					IsRedistributable: true,
				},
				Name: "pkg",
			},
			Documentation:     []*internal.Documentation{sample.Documentation("linux", "amd64", sample.DocContents)},
			Licenses:          sample.LicenseMetadata(),
			Imports:           []string{pkgPath},
			IsRedistributable: true,
		}},
	})

	for _, mp := range []string{modulePath1, modulePath2} {
		u := &internal.Unit{
			UnitMeta: internal.UnitMeta{
				Path: pkgPath,
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        mp,
					Version:           version,
					LatestVersion:     version,
					IsRedistributable: true,
				},
				Name: "b",
			},
			Documentation:     []*internal.Documentation{sample.Documentation("linux", "amd64", sample.DocContents)},
			IsRedistributable: true,
		}
		ds.MustInsertModule(t, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:    mp,
				Version:       version,
				LatestVersion: version,
			},
			Units: []*internal.Unit{u},
		})
	}

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       "v1.2.4",
			LatestVersion: "v1.2.4",
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:    "example.com",
					Version:       "v1.2.4",
					LatestVersion: "v1.2.4",
				},
				Name: "pkg",
			},
			Documentation:     []*internal.Documentation{sample.Documentation("linux", "amd64", sample.DocContents)},
			IsRedistributable: true,
		}},
	})

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com/ex",
			Version:       "v1.0.0",
			LatestVersion: "v1.0.0",
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: "example.com/ex/pkg",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:    "example.com/ex",
					Version:       "v1.0.0",
					LatestVersion: "v1.0.0",
				},
				Name: "pkg",
			},
			IsRedistributable: true,
			Documentation: []*internal.Documentation{sample.DocumentationWithExamples("linux", "amd64", "", `
			import "fmt"
			func Example() {
			fmt.Println("hello")
			// Output: hello
			}
			`)},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any // Can be *Package or *Error
		overrideDS internal.DataSource
	}{
		{
			name:       "basic metadata",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "ambiguous path",
			url:        "/v1/package/example.com/a/b?version=v1.2.3",
			wantStatus: http.StatusBadRequest,
			want: &Error{
				Code:    http.StatusBadRequest,
				Message: "ambiguous package path",
				Candidates: []Candidate{
					{ModulePath: "example.com/a/b", PackagePath: "example.com/a/b"},
					{ModulePath: "example.com/a", PackagePath: "example.com/a/b"},
				},
			},
		},
		{
			name:       "disambiguated path",
			url:        "/v1/package/example.com/a/b?version=v1.2.3&module=example.com/a",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          pkgPath,
				ModulePath:    modulePath1,
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "default build context",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "latest version",
			url:        "/v1/package/example.com/pkg?version=v1.2.4",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.4",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "doc",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=text",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package p\n\nPackage p is a package.\n\n# Links\n\n- pkg.go.dev, https://pkg.go.dev\n\nVARIABLES\n\nvar V int\n\n",
			},
		},
		{
			name:       "doc with examples",
			url:        "/v1/package/example.com/ex/pkg?version=v1.0.0&doc=text&examples=true",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/ex/pkg",
				ModulePath:    "example.com/ex",
				ModuleVersion: "v1.0.0",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package pkg\n\nPackage pkg is a package.\n\nExample:\n\t{\n\t\tfmt.Println(\"hello\")\n\t}\n\n\tOutput:\n\thello\n\n",
			},
		},
		{
			name:       "examples without doc (returns 400)",
			url:        "/v1/package/example.com/ex/pkg?version=v1.0.0&examples=true",
			wantStatus: http.StatusBadRequest,
			want: &Error{
				Code:    http.StatusBadRequest,
				Message: "examples require doc format to be specified",
			},
		},
		{
			name:       "package not found",
			url:        "/v1/package/nonexistent.com/pkg",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "doc without examples",
			url:        "/v1/package/example.com/ex/pkg?version=v1.0.0&doc=text&examples=false",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/ex/pkg",
				ModulePath:    "example.com/ex",
				ModuleVersion: "v1.0.0",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package pkg\n\nPackage pkg is a package.\n\n",
			},
		},
		{
			name:       "invalid doc format",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=invalid",
			wantStatus: http.StatusBadRequest,
			want: &Error{
				Code:    http.StatusBadRequest,
				Message: "bad doc format: need one of 'text', 'md', 'markdown' or 'html'",
			},
		},
		{
			name:       "empty doc format",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "",
			},
		},
		{
			name:       "licenses",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&licenses=true",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Licenses: []License{
					{
						Types:    []string{sample.LicenseType},
						FilePath: sample.LicenseFilePath,
						Contents: "Lorem Ipsum",
					},
				},
			},
		},
		{
			name:       "imports",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&imports=true",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Imports:       []string{pkgPath},
			},
		},
		{
			name:       "fallback prevention (false positive candidate)",
			url:        "/v1/package/example.com/a/b?version=v1.2.3",
			wantStatus: http.StatusBadRequest,
			want: &Error{
				Code:    http.StatusBadRequest,
				Message: "ambiguous package path",
				Candidates: []Candidate{
					{ModulePath: "example.com/a/b", PackagePath: "example.com/a/b"},
					{ModulePath: "example.com/a", PackagePath: "example.com/a/b"},
				},
			},
			overrideDS: &fallbackDataSource{
				DataSource: ds,
				fallbackMap: map[string]string{
					"example.com": "example.com/a/b", // simulate fallback
				},
			},
		},
		{
			name:       "deprecation filtering",
			url:        "/v1/package/example.com/a/b?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          pkgPath,
				ModulePath:    modulePath2, // picked because modulePath1 is deprecated
				ModuleVersion: version,
				Synopsis:      "Synopsis for " + modulePath2,
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
			overrideDS: func() internal.DataSource {
				newDS := fakedatasource.New()
				for _, mp := range []string{modulePath1, modulePath2} {
					u := &internal.Unit{
						UnitMeta: internal.UnitMeta{
							Path: pkgPath,
							ModuleInfo: internal.ModuleInfo{
								ModulePath:    mp,
								Version:       version,
								LatestVersion: version,
								Deprecated:    mp == modulePath1,
							},
							Name: "b",
						},
						Documentation: []*internal.Documentation{
							{
								GOOS:     "linux",
								GOARCH:   "amd64",
								Synopsis: "Synopsis for " + mp,
							},
						},
					}
					newDS.MustInsertModule(t, &internal.Module{
						ModuleInfo: internal.ModuleInfo{
							ModulePath:    mp,
							Version:       version,
							LatestVersion: version,
							Deprecated:    mp == modulePath1,
						},
						Units: []*internal.Unit{u},
					})
				}
				return newDS
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			var currentDS internal.DataSource = ds
			if test.overrideDS != nil {
				currentDS = test.overrideDS
			}
			if err := ServePackage(w, r, currentDS); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.want != nil {
				got, err := unmarshalResponse[Package](w.Body.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(Error{})); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func testServeModule(t *testing.T, ds internal.TestingDataSource) {

	const (
		modulePath = "example.com"
		version    = "v1.2.3"
	)

	mi1 := sample.ModuleInfo(modulePath, version)
	mi1.LatestVersion = "v1.2.4"
	mi1.HasGoMod = true

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: *mi1,
		Licenses:   sample.Licenses(),
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       modulePath,
				Name:       "pkg",
				ModuleInfo: *mi1,
			},
			Readme:            &internal.Readme{Filepath: "README.md", Contents: "Hello world"},
			Licenses:          sample.LicenseMetadata(),
			IsRedistributable: true,
		}},
	})

	mi2 := sample.ModuleInfo(modulePath, "v1.2.4")
	mi2.LatestVersion = "v1.2.4"
	mi2.HasGoMod = true

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: *mi2,
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       modulePath,
				Name:       "pkg",
				ModuleInfo: *mi2,
			},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any
	}{
		{
			name:       "basic module metadata",
			url:        "/v1/module/example.com?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:              modulePath,
				Version:           version,
				IsRedistributable: true,
				HasGoMod:          true,
				RepoURL:           "https://example.com",
			},
		},
		{
			name:       "latest module metadata",
			url:        "/v1/module/example.com?version=v1.2.4",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:              modulePath,
				Version:           "v1.2.4",
				IsLatest:          true,
				IsRedistributable: true,
				HasGoMod:          true,
				RepoURL:           "https://example.com",
			},
		},
		{
			name:       "bad version",
			url:        "/v1/module/example.com?version=nope",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "module not found",
			url:        "/v1/module/nonexistent.com",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/module/",
			wantStatus: http.StatusBadRequest,
			want:       &Error{Code: 400, Message: "missing module path"},
		},
		{
			name:       "module with readme",
			url:        "/v1/module/example.com?version=v1.2.3&readme=true",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:              modulePath,
				Version:           version,
				IsRedistributable: true,
				HasGoMod:          true,
				RepoURL:           "https://example.com",
				Readme: &Readme{
					Filepath: "README.md",
					Contents: "Hello world",
				},
			},
		},
		{
			name:       "module with licenses",
			url:        "/v1/module/example.com?version=v1.2.3&licenses=true",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:              modulePath,
				Version:           version,
				IsRedistributable: true,
				HasGoMod:          true,
				RepoURL:           "https://example.com",
				Licenses: []License{
					{
						Types:    []string{"MIT"},
						FilePath: "LICENSE",
						Contents: "Lorem Ipsum",
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeModule(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.want != nil {
				got, err := unmarshalResponse[Module](w.Body.Bytes())
				if err != nil {
					t.Fatalf("unmarshaling: %v", err)
				}
				if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(Error{})); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func testServeModuleVersions(t *testing.T, ds internal.TestingDataSource) {
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       "v1.0.0",
			LatestVersion: "v1.1.0",
		},
		Units: []*internal.Unit{{UnitMeta: internal.UnitMeta{
			Path: "example.com",
			Name: "pkg",
		}}},
	})
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       "v1.1.0",
			LatestVersion: "v1.1.0",
		},
		Units: []*internal.Unit{{UnitMeta: internal.UnitMeta{
			Path: "example.com",
			Name: "pkg",
		}}},
	})
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com/v2",
			Version:       "v2.0.0",
			LatestVersion: "v2.0.0",
		},
		Units: []*internal.Unit{{UnitMeta: internal.UnitMeta{
			Path: "example.com/v2",
			Name: "pkg",
		}}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		want       any
	}{
		{
			name:       "all versions (cross-major)",
			url:        "/v1/versions/example.com",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "module not found",
			url:        "/v1/versions/nonexistent.com",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/versions/",
			wantStatus: http.StatusBadRequest,
			want:       &Error{Code: 400, Message: "missing module path"},
		},
		{
			name:       "with limit",
			url:        "/v1/versions/example.com?limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "pagination",
			url:        "/v1/versions/example.com?limit=1&token=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeModuleVersions(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[internal.ModuleInfo]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
			}
		})
	}
}

func testServeModulePackages(t *testing.T, ds internal.TestingDataSource) {
	const (
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           version,
			LatestVersion:     version,
			IsRedistributable: true,
		},
		Units: []*internal.Unit{
			{
				UnitMeta: internal.UnitMeta{Path: modulePath, Name: "pkg1"},
				Documentation: []*internal.Documentation{
					sample.Documentation("linux", "amd64", sample.DocContents),
				},
				IsRedistributable: true,
			},
			{
				UnitMeta: internal.UnitMeta{Path: modulePath + "/sub", Name: "pkg2"},
				Documentation: []*internal.Documentation{
					func() *internal.Documentation {
						d := sample.Documentation("linux", "amd64", sample.DocContents)
						d.Synopsis = "Synopsis for name pkg2, path sub"
						return d
					}(),
				},
				IsRedistributable: true,
			},
		},
	})
	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		wantTotal  int
		wantToken  string
		want       any
	}{
		{
			name:       "all packages",
			url:        "/v1/packages/example.com?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantTotal:  2,
		},
		{
			name:       "module not found",
			url:        "/v1/packages/nonexistent.com?version=v1.0.0",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/packages/",
			wantStatus: http.StatusBadRequest,
			want:       &Error{Code: 400, Message: "missing module path"},
		},
		{
			name:       "filtering",
			url:        "/v1/packages/example.com?version=v1.0.0&filter=sub",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "filtering synopsis",
			url:        "/v1/packages/example.com?version=v1.0.0&filter=pkg2",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "limit and token",
			url:        "/v1/packages/example.com?version=v1.0.0&limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  2,
			wantToken:  "1",
		},
		{
			name:       "next page",
			url:        "/v1/packages/example.com?version=v1.0.0&limit=1&token=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeModulePackages(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Package]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
				if got.Total != test.wantTotal {
					t.Errorf("total = %d, want %d", got.Total, test.wantTotal)
				}
				if got.NextPageToken != test.wantToken {
					t.Errorf("token = %q, want %q", got.NextPageToken, test.wantToken)
				}
			}
		})
	}
}

func testServeSearch(t *testing.T, ds internal.TestingDataSource) {
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        "example.com",
			Version:           "v1.0.0",
			LatestVersion:     "v1.0.0",
			IsRedistributable: true,
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: "v1.0.0"},
				Name:       "pkg",
			},
			Documentation: []*internal.Documentation{
				sample.Documentation("linux", "amd64", sample.DocContents),
			},
			IsRedistributable: true,
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "basic search",
			url:        "/v1/search?q=synopsis",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "no results",
			url:        "/v1/search?q=nonexistent",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "missing query",
			url:        "/v1/search",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search with filter",
			url:        "/v1/search?q=synopsis&filter=example.com",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "search with non-matching filter",
			url:        "/v1/search?q=great&filter=nomatch",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeSearch(w, r, ds)
			if err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("%s: status = %d, want %d", test.name, w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[SearchResult]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("%s: json.Unmarshal: %v", test.name, err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("%s: count = %d, want %d", test.name, len(got.Items), test.wantCount)
				}
			}
		})
	}
}

func testServeSearchPagination(t *testing.T, ds internal.TestingDataSource) {
	for i := range 10 {
		pkgPath := "example.com/pkg" + strconv.Itoa(i)
		ds.MustInsertModule(t, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        pkgPath,
				Version:           "v1.0.0",
				LatestVersion:     "v1.0.0",
				IsRedistributable: true,
			},
			Units: []*internal.Unit{{
				UnitMeta: internal.UnitMeta{
					Path:       pkgPath,
					ModuleInfo: internal.ModuleInfo{ModulePath: pkgPath, Version: "v1.0.0"},
					Name:       "pkg",
				},
				IsRedistributable: true,
				Documentation: []*internal.Documentation{
					func() *internal.Documentation {
						d := sample.Documentation("linux", "amd64", sample.DocContents)
						d.Synopsis = fmt.Sprintf("Synopsis %d", i)
						return d
					}(),
				}},
			},
		})
	}

	for _, test := range []struct {
		name          string
		url           string
		wantCount     int
		wantTotal     int
		wantNextToken string
	}{
		{
			name:          "first page",
			url:           "/v1/search?q=Synopsis&limit=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "3",
		},
		{
			name:          "second page",
			url:           "/v1/search?q=Synopsis&limit=3&token=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "6",
		},
		{
			name:          "last page",
			url:           "/v1/search?q=Synopsis&limit=3&token=9",
			wantCount:     1,
			wantTotal:     10,
			wantNextToken: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeSearch(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var got PaginatedResponse[SearchResult]
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			if len(got.Items) != test.wantCount {
				t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
			}
			if got.Total != test.wantTotal {
				t.Errorf("total = %d, want %d", got.Total, test.wantTotal)
			}
			if got.NextPageToken != test.wantNextToken {
				t.Errorf("nextToken = %q, want %q", got.NextPageToken, test.wantNextToken)
			}
		})
	}
}

func testServePackageSymbols(t *testing.T, ds internal.TestingDataSource) {

	const (
		pkgPath    = "example.com/pkg"
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	sym := func(doc *internal.Documentation, name string) *internal.Symbol {
		return &internal.Symbol{
			SymbolMeta: internal.SymbolMeta{
				Name:    name,
				Kind:    internal.SymbolKindFunction,
				Section: internal.SymbolSectionFunctions,
			},
			GOOS:   doc.GOOS,
			GOARCH: doc.GOARCH,
		}
	}

	linuxDoc := sample.Documentation("linux", "amd64", sample.DocContents)
	linuxDoc.API = []*internal.Symbol{sym(linuxDoc, "LinuxSym"), sym(linuxDoc, "F")}
	winDoc := sample.Documentation("windows", "amd64", sample.DocContents)
	winDoc.API = []*internal.Symbol{sym(winDoc, "WindowsSym")}
	wasmDoc := sample.Documentation("js", "wasm", sample.DocContents)
	wasmDoc.API = []*internal.Symbol{sym(wasmDoc, "WasmSym")}

	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           version,
			LatestVersion:     version,
			IsRedistributable: true,
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       pkgPath,
				ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version},
				Name:       "pkg",
			},
			IsRedistributable: true,
			Documentation:     []*internal.Documentation{linuxDoc, winDoc, wasmDoc},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantNames  []string // sorted
		want       any
	}{
		{
			name:       "default best match (linux)",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantNames:  []string{"F", "LinuxSym"},
		},
		{
			name:       "package not found",
			url:        "/v1/symbols/nonexistent.com/pkg?version=v1.0.0",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing package path",
			url:        "/v1/symbols/",
			wantStatus: http.StatusBadRequest,
			want:       &Error{Code: 400, Message: "missing package path"},
		},
		{
			name:       "explicit linux",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=linux&goarch=amd64",
			wantStatus: http.StatusOK,
			wantNames:  []string{"F", "LinuxSym"},
		},
		{
			name:       "version latest",
			url:        "/v1/symbols/example.com/pkg?version=latest",
			wantStatus: http.StatusOK,
			wantNames:  []string{"F", "LinuxSym"},
		},
		{
			name:       "explicit windows",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=windows&goarch=amd64",
			wantStatus: http.StatusOK,
			wantNames:  []string{"WindowsSym"},
		},
		{
			name:       "explicit wasm",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=js&goarch=wasm",
			wantStatus: http.StatusOK,
			wantNames:  []string{"WasmSym"},
		},
		{
			name:       "not found build context",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=darwin&goarch=amd64",
			wantStatus: http.StatusNotFound,
			want: &Error{
				Code:    http.StatusNotFound,
				Message: "not found",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServePackageSymbols(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.want != nil {
				if want, ok := test.want.(*Error); ok {
					var got Error
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatalf("json.Unmarshal: %v", err)
					}
					if diff := cmp.Diff(want, &got, cmpopts.IgnoreUnexported(Error{})); diff != "" {
						t.Errorf("mismatch (-want +got):\n%s", diff)
					}
				}
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Symbol]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				var gotNames []string
				for _, it := range got.Items {
					gotNames = append(gotNames, it.Name)
				}
				slices.Sort(gotNames)
				if !slices.Equal(gotNames, test.wantNames) {
					t.Errorf("got names %q, want %q", gotNames, test.wantNames)
				}
			}
		})
	}
}

func testServePackageImportedBy(t *testing.T, ds internal.TestingDataSource) {

	const (
		pkgPath    = "example.com/pkg"
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	modInfo := internal.ModuleInfo{
		ModulePath:        modulePath,
		Version:           version,
		LatestVersion:     version,
		IsRedistributable: true,
	}
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: modInfo,
		Units: []*internal.Unit{
			{
				UnitMeta: internal.UnitMeta{
					Path:       pkgPath,
					ModuleInfo: modInfo,
					Name:       "pkg",
				},
				IsRedistributable: true,
			},
		},
	})

	modInfo2 := internal.ModuleInfo{
		ModulePath:        "example.com/mod",
		Version:           version,
		LatestVersion:     version,
		IsRedistributable: true,
	}
	ds.MustInsertModule(t, &internal.Module{
		ModuleInfo: modInfo2,
		Units: []*internal.Unit{
			{
				UnitMeta: internal.UnitMeta{
					Path:       pkgPath,
					ModuleInfo: modInfo2,
					Name:       "pkg",
				},
				IsRedistributable: true,
				Imports:           []string{pkgPath},
			},
		},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		want       any
	}{
		{
			name:       "all imported by",
			url:        "/v1/imported-by/example.com/pkg?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServePackageImportedBy(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PackageImportedBy
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.ImportedBy.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.ImportedBy.Items), test.wantCount)
				}
			}
		})
	}
}

func TestServeVulnerabilities(t *testing.T) {
	// This test doesn't need to run against a Postgres DB, because
	// vulnerabilities are not read from the database.
	vc, err := vuln.NewInMemoryClient([]*osv.Entry{
		{
			ID:      "VULN-1",
			Summary: "Vulnerability 1",
			Affected: []osv.Affected{
				{
					Module: osv.Module{Path: "example.com"},
					Ranges: []osv.Range{{Type: osv.RangeTypeSemver, Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.1.0"}}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		want       any
	}{
		{
			name:       "all vulns",
			url:        "/v1/vulns/example.com?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "no vulns",
			url:        "/v1/vulns/example.com?version=v1.2.0",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeVulnerabilities(vc)(w, r, nil); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Vulnerability]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
			}
		})
	}
}

// unmarshalResponse unmarshals an API response into either
// a *T or an *Error.
func unmarshalResponse[T any](data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var t T
	err1 := d.Decode(&t)
	if err1 == nil {
		return &t, nil
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var e Error
	err2 := d.Decode(&e)
	if err2 == nil {
		return &e, nil
	}
	return nil, errors.Join(err1, err2)
}

func TestCacheControl(t *testing.T) {
	// This test doesn't need to run against a Postgres DB, because
	// it's concerned only with headers.
	ds := fakedatasource.New()
	const modulePath = "example.com"
	for _, v := range []string{"v1.0.0", "master"} {
		ds.MustInsertModule(t, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: modulePath,
				Version:    v,
			},
			Units: []*internal.Unit{{
				UnitMeta: internal.UnitMeta{
					Path: modulePath,
					ModuleInfo: internal.ModuleInfo{
						ModulePath: modulePath,
						Version:    v,
					},
				},
			}},
		})
	}

	for _, test := range []struct {
		version string
		want    string
	}{
		{"v1.0.0", "public, max-age=10800"},
		{"latest", "public, max-age=3600"},
		{"master", "public, max-age=3600"},
		{"", "public, max-age=3600"},
	} {
		t.Run(test.version, func(t *testing.T) {
			url := "/v1/module/" + modulePath
			if test.version != "" {
				url += "?version=" + test.version
			}
			r := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			if err := ServeModule(w, r, ds); err != nil {
				t.Fatal(err)
			}

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			got := w.Header().Get("Cache-Control")
			if got != test.want {
				t.Errorf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}
