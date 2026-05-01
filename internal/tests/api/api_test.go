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
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/api"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
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

// modinfo creates a ModuleInfo with the given module path
// and version. It sets LatestVersion to version.
func modinfo(path, version string) internal.ModuleInfo {
	return internal.ModuleInfo{
		ModulePath:        path,
		Version:           version,
		LatestVersion:     version,
		IsRedistributable: true,
	}
}

// Module creates a module with the given ModuleInfo and units.
// The units must not have been previously used.
func module(t *testing.T, mi internal.ModuleInfo, units ...*internal.Unit) *internal.Module {
	for _, u := range units {
		// If Name is already set, then this unit was used to
		// build another module, and that's bad.
		if u.Name != "" {
			t.Fatal("unit used in two modules")
		}
		u.ModuleInfo = mi
		// Change relative to absolute path.
		u.Path = path.Join(mi.ModulePath, u.Path)
		// Name is last component of path.
		u.Name = u.Path[strings.LastIndexByte(u.Path, '/')+1:]
	}
	return &internal.Module{
		ModuleInfo: mi,
		Licenses:   sample.Licenses(),
		Units:      units,
	}
}

// unit constructs a Unit with the given relative path and documentation.
// The path is relative to a module path; the full import path
// will be constructed when the Unit is added to a module in
// the [module] function.
func unit(relativePath string, doc ...*internal.Documentation) *internal.Unit {
	return &internal.Unit{
		UnitMeta: internal.UnitMeta{
			Path: relativePath, // expanded in the module function
			// ModuleInfo and Name set in the module function.
		},
		Licenses:          sample.LicenseMetadata(),
		IsRedistributable: true,
		Documentation:     doc,
	}
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
		version       = "v1.2.3"
		latestVersion = "v1.2.4"
	)
	u := unit("pkg", sample.Documentation("linux", "amd64", sample.DocContents))
	u.Imports = []string{"example.com/a/b"}
	mi := modinfo("example.com", version)
	mi.LatestVersion = latestVersion
	ds.MustInsertModule(t, module(t, mi, u))

	ds.MustInsertModule(t, module(t, modinfo("example.com/a", version), unit("b", sample.Documentation("linux", "amd64", sample.DocContents))))
	ds.MustInsertModule(t, module(t, modinfo("example.com/a/b", version), unit("")))
	ds.MustInsertModule(t, module(t, modinfo("example.com", latestVersion),
		unit("pkg", sample.DocumentationWithTest("linux", "amd64", `
		// Package p is a package.
		package p
		var V int
		`, `
		package p
	    func Example() {
			fmt.Println("hello")
            // Output: hello
        }`),
		)))

	// Deprecation.
	// The fake data source uses ModuleInfo.Deprecated, but the DB
	// requires a go.mod file.
	modInfo := modinfo("example.com/d/e", version)
	modInfo.Deprecated = true
	ds.MustInsertModuleGoMod(context.TODO(), t, module(t, modInfo, unit("e")), `module example.com/d/e // Deprecated: bad`)
	// The DB needs the above go.mod contents to know that the module
	// is deprecated. It doesn't look at ModuleInfo.Deprecated.
	ds.MustInsertModule(t, module(t, modinfo("example.com/d", version), unit("e")))

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any // Can be *Package or *Error
		overrideDS internal.DataSource
	}{
		{
			name:       "missing package path",
			url:        "/v1/package/",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "missing package path",
			},
		},
		{
			name:       "basic metadata",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
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
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "ambiguous package path",
				Candidates: []api.Candidate{
					{ModulePath: "example.com/a/b", PackagePath: "example.com/a/b"},
					{ModulePath: "example.com/a", PackagePath: "example.com/a/b"},
				},
			},
		},
		{
			name:       "disambiguated path",
			url:        "/v1/package/example.com/a/b?version=v1.2.3&module=example.com/a",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/a/b",
				ModulePath:    "example.com/a",
				ModuleVersion: "v1.2.3",
				IsLatest:      true,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "default build context",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
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
			want: &api.Package{
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
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package p\n\nPackage p is a package.\n\n# Links\n\n- pkg.go.dev, https://pkg.go.dev\n\nVARIABLES\n\nvar V int\n\n",
			},
		},
		{
			name:       "doc with examples",
			url:        "/v1/package/example.com/pkg?version=v1.2.4&doc=text&examples=true",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.4",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package p\n\nPackage p is a package.\n\nExample:\n\t{\n\t\tfmt.Println(\"hello\")\n\t}\n\n\tOutput:\n\thello\n\nVARIABLES\n\nvar V int\n\n",
			},
		},
		{
			name:       "examples without doc",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&examples=true",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "examples require doc format to be specified",
			},
		},
		{
			name:       "package not found",
			url:        "/v1/package/nonexistent.com/pkg",
			wantStatus: http.StatusNotFound,
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "doc without examples",
			url:        "/v1/package/example.com/pkg?version=v1.2.4&doc=text&examples=false",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.4",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package p\n\nPackage p is a package.\n\nVARIABLES\n\nvar V int\n\n",
			},
		},
		{
			name:       "invalid doc format",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=invalid",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "bad doc format: need one of 'text', 'md', 'markdown' or 'html'",
			},
		},
		{
			name:       "empty doc format",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
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
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Licenses: []api.License{
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
			want: &api.Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.3",
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
				Imports:       []string{"example.com/a/b"},
			},
		},
		{
			name:       "fallback prevention (false positive candidate)",
			url:        "/v1/package/example.com/a/b?version=v1.2.3",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "ambiguous package path",
				Candidates: []api.Candidate{
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
			url:        "/v1/package/example.com/d/e?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &api.Package{
				Path:          "example.com/d/e",
				ModulePath:    "example.com/d", // picked because example.com/d/e is deprecated
				ModuleVersion: "v1.2.3",
				IsLatest:      true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			var currentDS internal.DataSource = ds
			if test.overrideDS != nil {
				currentDS = test.overrideDS
			}
			if err := api.ServePackage(w, r, currentDS); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.want != nil {
				got, err := unmarshalResponse[api.Package](w.Body.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(api.Error{})); diff != "" {
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

	mi1 := modinfo(modulePath, version)
	mi1.LatestVersion = "v1.2.4"
	mi1.HasGoMod = true

	u := unit("")
	u.Readme = &internal.Readme{Filepath: "README.md", Contents: "Hello world"}
	ds.MustInsertModule(t, module(t, mi1, u))

	mi2 := modinfo(modulePath, "v1.2.4")
	mi2.HasGoMod = true

	ds.MustInsertModule(t, module(t, mi2, unit("")))

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any
	}{
		{
			name:       "invalid query parameter",
			url:        "/v1/module/example.com?licenses=invalid",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: `invalid boolean value "invalid" for licenses: strconv.ParseBool: parsing "invalid": invalid syntax`,
			},
		},
		{
			name:       "basic module metadata",
			url:        "/v1/module/example.com?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &api.Module{
				Path:              modulePath,
				Version:           "v1.2.3",
				IsRedistributable: true,
				HasGoMod:          true,
			},
		},
		{
			name:       "latest module metadata",
			url:        "/v1/module/example.com?version=v1.2.4",
			wantStatus: http.StatusOK,
			want: &api.Module{
				Path:              modulePath,
				Version:           "v1.2.4",
				IsLatest:          true,
				IsRedistributable: true,
				HasGoMod:          true,
			},
		},
		{
			name:       "bad version",
			url:        "/v1/module/example.com?version=nope",
			wantStatus: http.StatusNotFound,
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "module not found",
			url:        "/v1/module/nonexistent.com",
			wantStatus: http.StatusNotFound,
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/module/",
			wantStatus: http.StatusBadRequest,
			want:       &api.Error{Code: 400, Message: "missing module path"},
		},
		{
			name:       "module with readme",
			url:        "/v1/module/example.com?version=v1.2.3&readme=true",
			wantStatus: http.StatusOK,
			want: &api.Module{
				Path:              modulePath,
				Version:           "v1.2.3",
				IsRedistributable: true,
				HasGoMod:          true,
				Readme: &api.Readme{
					Filepath: "README.md",
					Contents: "Hello world",
				},
			},
		},
		{
			name:       "module with licenses",
			url:        "/v1/module/example.com?version=v1.2.3&licenses=true",
			wantStatus: http.StatusOK,
			want: &api.Module{
				Path:              modulePath,
				Version:           "v1.2.3",
				IsRedistributable: true,
				HasGoMod:          true,
				Licenses: []api.License{
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

			if err := api.ServeModule(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.want != nil {
				got, err := unmarshalResponse[api.Module](w.Body.Bytes())
				if err != nil {
					t.Fatalf("unmarshaling: %v", err)
				}
				if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(api.Error{})); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func testServeModuleVersions(t *testing.T, ds internal.TestingDataSource) {
	newMod := func(path, version, latest string) *internal.Module {
		mi := modinfo(path, version)
		mi.LatestVersion = latest
		return module(t, mi, unit(""))
	}
	ds.MustInsertModule(t, newMod("example.com", "v1.0.0", "v1.1.0"))
	ds.MustInsertModule(t, newMod("example.com", "v1.1.0", "v1.1.0"))
	ds.MustInsertModule(t, newMod("example.com/v2", "v2.0.0", "v2.0.0"))

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
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/versions/",
			wantStatus: http.StatusBadRequest,
			want:       &api.Error{Code: 400, Message: "missing module path"},
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

			if err := api.ServeModuleVersions(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got api.PaginatedResponse[internal.ModuleInfo]
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
	d := sample.Documentation("linux", "amd64", sample.DocContents)
	d.Synopsis = "Synopsis for name pkg2, path sub"
	ds.MustInsertModule(t,
		module(t, modinfo("example.com", "v1.2.3"),
			unit("", sample.Documentation("linux", "amd64", sample.DocContents)),
			unit("sub", d)))

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
			url:        "/v1/packages/example.com?version=v1.2.3",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantTotal:  2,
		},
		{
			name:       "latest",
			url:        "/v1/packages/example.com",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantTotal:  2,
		},
		{
			name:       "module not found",
			url:        "/v1/packages/nonexistent.com?version=v1.2.3",
			wantStatus: http.StatusNotFound,
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing module path",
			url:        "/v1/packages/",
			wantStatus: http.StatusBadRequest,
			want:       &api.Error{Code: 400, Message: "missing module path"},
		},
		{
			name:       "filtering",
			url:        "/v1/packages/example.com?version=v1.2.3&filter=sub",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "filtering synopsis",
			url:        "/v1/packages/example.com?version=v1.2.3&filter=pkg2",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "limit and token",
			url:        "/v1/packages/example.com?version=v1.2.3&limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  2,
			wantToken:  "1",
		},
		{
			name:       "next page",
			url:        "/v1/packages/example.com?version=v1.2.3&limit=1&token=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := api.ServeModulePackages(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got api.PaginatedResponse[api.Package]
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
	ds.MustInsertModule(t, module(t, modinfo("example.com", "v1.0.0"),
		unit("pkg", sample.Documentation("linux", "amd64", sample.DocContents))))

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

			err := api.ServeSearch(w, r, ds)
			if err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("%s: status = %d, want %d", test.name, w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got api.PaginatedResponse[api.SearchResult]
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
	doc := sample.Documentation("linux", "amd64", sample.DocContents)
	for i := range 10 {
		modPath := fmt.Sprintf("example.com/m%d", i)
		ds.MustInsertModule(t, module(t, modinfo(modPath, "v1.0.0"), unit("pkg", doc)))
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
			url:           "/v1/search?q=synopsis&limit=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "3",
		},
		{
			name:          "second page",
			url:           "/v1/search?q=synopsis&limit=3&token=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "6",
		},
		{
			name:          "last page",
			url:           "/v1/search?q=synopsis&limit=3&token=9",
			wantCount:     1,
			wantTotal:     10,
			wantNextToken: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := api.ServeSearch(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var got api.PaginatedResponse[api.SearchResult]
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
	ds.MustInsertModule(t,
		module(t, modinfo("example.com", "v1.0.0"),
			unit("pkg", linuxDoc, winDoc, wasmDoc)))

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
			want:       &api.Error{Code: 404, Message: "not found"},
		},
		{
			name:       "missing package path",
			url:        "/v1/symbols/",
			wantStatus: http.StatusBadRequest,
			want:       &api.Error{Code: 400, Message: "missing package path"},
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
			want: &api.Error{
				Code:    http.StatusNotFound,
				Message: "not found",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := api.ServePackageSymbols(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.want != nil {
				if want, ok := test.want.(*api.Error); ok {
					var got api.Error
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatalf("json.Unmarshal: %v", err)
					}
					if diff := cmp.Diff(want, &got, cmpopts.IgnoreUnexported(api.Error{})); diff != "" {
						t.Errorf("mismatch (-want +got):\n%s", diff)
					}
				}
			}

			if test.wantStatus == http.StatusOK {
				var got api.PackageSymbols
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				var gotNames []string
				for _, it := range got.Symbols.Items {
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

	ds.MustInsertModule(t, module(t, modinfo("example.com", "v1.2.3"), unit("pkg")))

	u := unit("pkg")
	u.Imports = []string{"example.com/pkg"}
	ds.MustInsertModule(t, module(t, modinfo("example.com/mod", "v1.2.3"), u))

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		want       any
	}{
		{
			name:       "missing package path",
			url:        "/v1/imported-by/",
			wantStatus: http.StatusBadRequest,
			want: &api.Error{
				Code:    http.StatusBadRequest,
				Message: "missing package path",
			},
		},
		{
			name:       "all imported by",
			url:        "/v1/imported-by/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := api.ServePackageImportedBy(w, r, ds); err != nil {
				api.ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got api.PackageImportedBy
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
	var e api.Error
	err2 := d.Decode(&e)
	if err2 == nil {
		return &e, nil
	}
	return nil, errors.Join(err1, err2)
}
