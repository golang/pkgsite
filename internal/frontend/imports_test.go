// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/frontend/templates"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/static"
)

func TestFetchImportsDetails(t *testing.T) {
	for _, test := range []struct {
		name        string
		imports     []string
		wantDetails *ImportsDetails
	}{
		{
			name: "want imports details with standard and internal",
			imports: []string{
				"pa.th/import/1",
				sample.PackagePath,
				"context",
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1"},
				InternalImports: []string{sample.PackagePath},
				StdLib:          []string{"context"},
			},
		},
		{
			name:    "want expected imports details with multiple",
			imports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
				StdLib:          nil,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fds := fakedatasource.New()

			ctx := context.Background()

			module := sample.Module(sample.ModulePath, sample.VersionString, sample.Suffix)
			// The first unit is the module and the second one is the package.
			pkg := module.Units[1]
			pkg.Imports = test.imports

			fds.MustInsertModule(t, module)

			got, err := fetchImportsDetails(ctx, fds, pkg.Path, pkg.ModulePath, pkg.Version)
			if err != nil {
				t.Fatalf("fetchImportsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					module.Units[1].Path, module.Version, got, err, test.wantDetails)
			}

			test.wantDetails.ModulePath = module.ModulePath
			if diff := cmp.Diff(test.wantDetails, got); diff != "" {
				t.Errorf("fetchImportsDetails(ctx, %q, %q) mismatch (-want +got):\n%s", module.Units[1].Path, module.Version, diff)
			}
		})
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	fds := fakedatasource.New()
	ctx := context.Background()

	newModule := func(modPath string, pkgs ...*internal.Unit) *internal.Module {
		m := sample.Module(modPath, sample.VersionString)
		for _, p := range pkgs {
			sample.AddUnit(m, p)
		}
		return m
	}

	pkg1 := sample.UnitForPackage("path.to/foo/bar", "path.to/foo", sample.VersionString, "bar", true)
	pkg2 := sample.UnitForPackage("path2.to/foo/bar2", "path2.to/foo", sample.VersionString, "bar2", true)
	pkg2.Imports = []string{pkg1.Path}

	pkg3 := sample.UnitForPackage("path3.to/foo/bar3", "path3.to/foor", sample.VersionString, "bar3", true)
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	testModules := []*internal.Module{
		newModule("path.to/foo", pkg1),
		newModule("path2.to/foo", pkg2),
		newModule("path3.to/foo", pkg3),
	}

	for _, m := range testModules {
		fds.MustInsertModule(t, m)
	}

	tests := []struct {
		pkg         *internal.Unit
		wantDetails *ImportedByDetails
	}{
		{
			pkg: pkg3,
			wantDetails: &ImportedByDetails{
				PackageNumImportedByDisplay: "0",
				ModuleNumImportedByDisplay:  "0",
			},
		},
		{
			pkg: pkg2,
			wantDetails: &ImportedByDetails{
				ImportedBy:                  []*Section{{Prefix: pkg3.Path, NumLines: 0}},
				PackageNumImportedByDisplay: "1",
				PackageTotal:                1,
				ModuleNumImportedByDisplay:  "1",
				ModuleTotal:                 1,
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ImportedBy: []*Section{
					{Prefix: pkg2.Path, NumLines: 0},
					{Prefix: pkg3.Path, NumLines: 0},
				},
				PackageNumImportedByDisplay: "2",
				PackageTotal:                2,
				ModuleNumImportedByDisplay:  "2",
				ModuleTotal:                 2,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.pkg.Path, func(t *testing.T) {
			otherVersion := newModule(path.Dir(test.pkg.Path), test.pkg)
			otherVersion.Version = "v1.0.5"
			pkg := otherVersion.Units[1]
			checkFetchImportedByDetails(ctx, fds, t, pkg, test.wantDetails)
		})
	}

	t.Run("unknown module count", func(t *testing.T) {
		checkFetchImportedByDetails(ctx, unknownModuleCountDB{fds}, t, pkg2, &ImportedByDetails{
			ImportedBy:                  []*Section{{Prefix: pkg3.Path, NumLines: 0}},
			PackageNumImportedByDisplay: "1",
			PackageTotal:                1,
			ModuleNumImportedByDisplay:  "not yet determined",
			ModuleTotal:                 0,
		})
	})
}

type unknownModuleCountDB struct {
	*fakedatasource.FakeDataSource
}

func (db unknownModuleCountDB) GetImportedByCounts(ctx context.Context, pkgPath, modulePath string) (internal.ImportedByCounts, error) {
	c, err := db.FakeDataSource.GetImportedByCounts(ctx, pkgPath, modulePath)
	if err != nil {
		return c, err
	}
	c.ModulesUpdatedAt = time.Time{}
	c.Modules = 0
	return c, nil
}

func TestFetchImportedByDetails_ExceedsLimit(t *testing.T) {
	fds := fakedatasource.New()
	ctx := context.Background()

	old := importedByLimit
	importedByLimit = 3
	defer func() { importedByLimit = old }()

	m := sample.Module("m.com/a", sample.VersionString, "foo")
	fds.MustInsertModule(t, m)
	for _, mod := range []string{"m1.com/a", "m2.com/a", "m3.com/a"} {
		m2 := sample.Module(mod, sample.VersionString, "p")
		m2.Packages()[0].Imports = []string{"m.com/a/foo"}
		fds.MustInsertModule(t, m2)
	}
	wantDetails := &ImportedByDetails{
		ModulePath: "m.com/a",
		ImportedBy: []*Section{
			{Prefix: "m1.com/a/p"},
			{Prefix: "m2.com/a/p"},
		},

		PackageNumImportedByDisplay: "3 (displaying more than 2 packages, including internal and invalid packages)",
		PackageTotal:                3,
		ModuleNumImportedByDisplay:  "3",
		ModuleTotal:                 3,
	}
	checkFetchImportedByDetails(ctx, fds, t, m.Packages()[0], wantDetails)
}

func checkFetchImportedByDetails(ctx context.Context, ds internal.DataSource, t *testing.T, pkg *internal.Unit, wantDetails *ImportedByDetails) {
	got, err := fetchImportedByDetails(ctx, ds, pkg.Path, pkg.ModulePath)
	if err != nil {
		t.Fatalf("fetchImportedByDetails(ctx, db, %q) = %v err = %v, want %v",
			pkg.Path, got, err, wantDetails)
	}
	wantDetails.ModulePath = pkg.ModulePath
	if diff := cmp.Diff(wantDetails, got); diff != "" {
		t.Errorf("fetchImportedByDetails(ctx, db, %q) mismatch (-want +got):\n%s", pkg.Path, diff)
	}
}

func TestImportedByTemplate(t *testing.T) {
	ts, err := templates.ParsePageTemplates(template.TrustedFSFromEmbed(static.FS))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := ts["unit/importedby"].Lookup("importedby")
	if tmpl == nil {
		t.Fatal("missing importedby template")
	}

	for _, test := range []struct {
		name        string
		details     ImportedByDetails
		wantStrings []string
		wantMissing []string
	}{
		{
			name: "no importers",
			details: ImportedByDetails{
				PackageNumImportedByDisplay: "0",
				ModuleNumImportedByDisplay:  "0",
			},
			wantStrings: []string{"No known importers for this package!"},
			wantMissing: []string{"Known importing", "ImportedBy-list"},
		},
		{
			name: "internal importer only (1 module, 0 external packages)",
			details: ImportedByDetails{
				PackageTotal:                0,
				PackageNumImportedByDisplay: "0",
				ModuleTotal:                 1,
				ModuleNumImportedByDisplay:  "1",
			},
			wantStrings: []string{
				"<strong>Known importing modules:</strong> 1",
				"<strong>Known importing packages:</strong> 0",
			},
			wantMissing: []string{"No known importers", "ImportedBy-list"},
		},
		{
			name: "1 module and 1 package",
			details: ImportedByDetails{
				ImportedBy:                  []*Section{{Prefix: "a.com/p"}},
				PackageTotal:                1,
				PackageNumImportedByDisplay: "1",
				ModuleTotal:                 1,
				ModuleNumImportedByDisplay:  "1",
			},
			wantStrings: []string{
				"<strong>Known importing modules:</strong> 1",
				"<strong>Known importing packages:</strong> 1",
				"a.com/p",
			},
		},
		{
			name: "multiple modules and packages",
			details: ImportedByDetails{
				ImportedBy: []*Section{
					{Prefix: "a.com/p"},
					{Prefix: "b.com/p"},
				},
				PackageTotal:                2,
				PackageNumImportedByDisplay: "2",
				ModuleTotal:                 2,
				ModuleNumImportedByDisplay:  "2",
			},
			wantStrings: []string{
				"<strong>Known importing modules:</strong> 2",
				"<strong>Known importing packages:</strong> 2",
				"a.com/p",
				"b.com/p",
			},
		},
		{
			name: "unknown module count with external package",
			details: ImportedByDetails{
				ImportedBy:                  []*Section{{Prefix: "a.com/p"}},
				PackageTotal:                1,
				PackageNumImportedByDisplay: "1",
				ModuleTotal:                 0,
				ModuleNumImportedByDisplay:  "not yet determined",
			},
			wantStrings: []string{
				"<strong>Known importing modules:</strong> not yet determined",
				"<strong>Known importing packages:</strong> 1",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, test.details); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			for _, w := range test.wantStrings {
				if !strings.Contains(got, w) {
					t.Errorf("got:\n%s\ndid not contain %q", got, w)
				}
			}
			for _, m := range test.wantMissing {
				if strings.Contains(got, m) {
					t.Errorf("got:\n%s\nunexpectedly contained %q", got, m)
				}
			}
		})
	}
}
