// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
)

// TabSettings defines tab-specific metadata.
type TabSettings struct {
	// Name is the tab name used in the URL.
	Name string

	// DisplayName is the formatted tab name.
	DisplayName string

	// AlwaysShowDetails defines whether the tab content can be shown even if the
	// package is not determined to be redistributable.
	AlwaysShowDetails bool

	// TemplateName is the name of the template used to render the
	// corresponding tab, as defined in Server.templates.
	TemplateName string

	// Disabled indicates whether a tab should be displayed as disabled.
	Disabled bool
}

var (
	packageTabSettings = []TabSettings{
		{
			Name:         tabDoc,
			DisplayName:  "Doc",
			TemplateName: "pkg_doc.tmpl",
		},
		{
			Name:              tabOverview,
			AlwaysShowDetails: true,
			DisplayName:       "Overview",
			TemplateName:      "overview.tmpl",
		},
		{
			Name:              tabSubdirectories,
			AlwaysShowDetails: true,
			DisplayName:       "Subdirectories",
			TemplateName:      "subdirectories.tmpl",
		},
		{
			Name:              tabVersions,
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:              tabImports,
			DisplayName:       "Imports",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_imports.tmpl",
		},
		{
			Name:              tabImportedBy,
			DisplayName:       "Imported By",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_importedby.tmpl",
		},
		{
			Name:         tabLicenses,
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	packageTabLookup = make(map[string]TabSettings)

	directoryTabSettings = make([]TabSettings, len(packageTabSettings))
	directoryTabLookup   = make(map[string]TabSettings)

	moduleTabSettings = []TabSettings{
		{
			Name:              tabOverview,
			AlwaysShowDetails: true,
			DisplayName:       "Overview",
			TemplateName:      "overview.tmpl",
		},
		{
			Name:              "packages",
			AlwaysShowDetails: true,
			DisplayName:       "Packages",
			TemplateName:      "subdirectories.tmpl",
		},
		{
			Name:              tabVersions,
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:         tabLicenses,
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	moduleTabLookup = make(map[string]TabSettings)
)

// validDirectoryTabs indicates if a tab is enabled in the directory view.
var validDirectoryTabs = map[string]bool{
	tabLicenses:       true,
	tabOverview:       true,
	tabSubdirectories: true,
}

func init() {
	for i, ts := range packageTabSettings {
		// The directory view uses the same design as the packages view
		// for visual consistency, but some tabs don't make sense, so
		// we disable them.
		if !validDirectoryTabs[ts.Name] {
			ts.Disabled = true
		}
		directoryTabSettings[i] = ts
	}
	for _, d := range packageTabSettings {
		packageTabLookup[d.Name] = d
	}
	for _, d := range directoryTabSettings {
		directoryTabLookup[d.Name] = d
	}
	for _, d := range moduleTabSettings {
		moduleTabLookup[d.Name] = d
	}
}

const (
	tabDoc            = "doc"
	tabOverview       = "overview"
	tabSubdirectories = "subdirectories"
	tabVersions       = "versions"
	tabImports        = "imports"
	tabImportedBy     = "importedby"
	tabLicenses       = "licenses"
)

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForPackage(r *http.Request, tab string, ds internal.DataSource, vdir *internal.VersionedDirectory) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case tabDoc:
		return fetchDocumentationDetails(vdir.Package.Documentation), nil
	case tabOverview:
		return fetchPackageOverviewDetails(ctx, vdir, urlIsVersioned(r.URL))
	case tabSubdirectories:
		return fetchDirectoryDetails(ctx, ds, vdir, false)
	case tabVersions:
		return fetchVersionsDetails(ctx, ds, vdir.Path, vdir.V1Path, vdir.ModulePath)
	case tabImports:
		return fetchImportsDetails(ctx, ds, vdir.Path, vdir.ModulePath, vdir.Version)
	case tabImportedBy:
		return fetchImportedByDetails(ctx, ds, vdir.Path, vdir.ModulePath)
	case tabLicenses:
		return fetchLicensesDetails(ctx, ds, vdir.Path, vdir.ModulePath, vdir.Version)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// fetchDetailsForModule returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForModule(r *http.Request, tab string, ds internal.DataSource, mi *internal.ModuleInfo, licenses []*licenses.License, readme *internal.Readme) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case "packages":
		if isActiveUseDirectories(ctx) {
			vdir := &internal.VersionedDirectory{
				ModuleInfo: *mi,
				Directory: internal.Directory{
					DirectoryMeta: internal.DirectoryMeta{
						Path:              mi.ModulePath,
						V1Path:            mi.SeriesPath(),
						IsRedistributable: mi.IsRedistributable,
						Licenses:          licensesToMetadatas(licenses),
					},
					Readme: readme,
				},
			}
			return fetchDirectoryDetails(ctx, ds, vdir, true)
		}
		return legacyFetchDirectoryDetails(ctx, ds, mi.ModulePath, mi, licensesToMetadatas(licenses), true)
	case tabLicenses:
		return &LicensesDetails{Licenses: transformLicenses(mi.ModulePath, mi.Version, licenses)}, nil
	case tabVersions:
		return fetchModuleVersionsDetails(ctx, ds, mi.ModulePath)
	case tabOverview:
		return constructOverviewDetails(ctx, mi, readme, mi.IsRedistributable, urlIsVersioned(r.URL))
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// fetchDetailsForDirectory returns tab details by delegating to the correct
// detail handler.
func fetchDetailsForDirectory(r *http.Request, tab string, ds internal.DataSource, vdir *internal.VersionedDirectory) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case tabOverview:
		return constructOverviewDetails(ctx, &vdir.ModuleInfo, vdir.Readme, vdir.IsRedistributable, urlIsVersioned(r.URL))
	case tabSubdirectories:
		return fetchDirectoryDetails(ctx, ds, vdir, false)
	case tabLicenses:
		return fetchLicensesDetails(ctx, ds, vdir.Path, vdir.ModulePath, vdir.Version)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// legacyFetchDetailsForDirectory returns tab details by delegating to the correct
// detail handler.
func legacyFetchDetailsForDirectory(r *http.Request, tab string, dir *internal.LegacyDirectory, licenses []*licenses.License) (interface{}, error) {
	switch tab {
	case tabOverview:
		readme := &internal.Readme{Filepath: dir.LegacyReadmeFilePath, Contents: dir.LegacyReadmeContents}
		return constructOverviewDetails(r.Context(), &dir.ModuleInfo, readme, dir.LegacyModuleInfo.IsRedistributable, urlIsVersioned(r.URL))
	case tabSubdirectories:
		// Ideally we would just use fetchDirectoryDetails here so that it
		// follows the same code path as fetchDetailsForModule and
		// fetchDetailsForPackage. However, since we already have the directory
		// and licenses info, it doesn't make sense to call
		// postgres.GetDirectory again.
		return legacyCreateDirectory(dir, licensesToMetadatas(licenses), false)
	case tabLicenses:
		return &LicensesDetails{Licenses: transformLicenses(dir.ModulePath, dir.Version, licenses)}, nil
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// legacyFetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func legacyFetchDetailsForPackage(r *http.Request, tab string, ds internal.DataSource, pkg *internal.LegacyVersionedPackage) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case tabDoc:
		return legacyFetchDocumentationDetails(pkg), nil
	case tabVersions:
		return legacyFetchPackageVersionsDetails(ctx, ds, pkg.Path, pkg.V1Path, pkg.ModulePath)
	case tabSubdirectories:
		return legacyFetchDirectoryDetails(ctx, ds, pkg.Path, &pkg.ModuleInfo, pkg.Licenses, false)
	case tabImports:
		return fetchImportsDetails(ctx, ds, pkg.Path, pkg.ModulePath, pkg.Version)
	case tabImportedBy:
		db, ok := ds.(*postgres.DB)
		if !ok {
			// The proxydatasource does not support the imported by page.
			return nil, proxydatasourceNotSupportedErr()
		}
		return fetchImportedByDetails(ctx, db, pkg.Path, pkg.ModulePath)
	case tabLicenses:
		return legacyFetchPackageLicensesDetails(ctx, ds, pkg.Path, pkg.ModulePath, pkg.Version)
	case tabOverview:
		return legacyFetchPackageOverviewDetails(ctx, pkg, urlIsVersioned(r.URL))
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

func urlIsVersioned(url *url.URL) bool {
	return strings.ContainsRune(url.Path, '@')
}
