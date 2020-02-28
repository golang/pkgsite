// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/licenses"
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
			Name:         "doc",
			DisplayName:  "Doc",
			TemplateName: "pkg_doc.tmpl",
		},
		{
			Name:              "overview",
			AlwaysShowDetails: true,
			DisplayName:       "Overview",
			TemplateName:      "overview.tmpl",
		},
		{
			Name:              "subdirectories",
			AlwaysShowDetails: true,
			DisplayName:       "Subdirectories",
			TemplateName:      "subdirectories.tmpl",
		},
		{
			Name:              "versions",
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:              "imports",
			DisplayName:       "Imports",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_imports.tmpl",
		},
		{
			Name:              "importedby",
			DisplayName:       "Imported By",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_importedby.tmpl",
		},
		{
			Name:         "licenses",
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	packageTabLookup = make(map[string]TabSettings)

	directoryTabSettings = make([]TabSettings, len(packageTabSettings))
	directoryTabLookup   = make(map[string]TabSettings)

	moduleTabSettings = []TabSettings{
		{
			Name:              "overview",
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
			Name:              "versions",
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:         "licenses",
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	moduleTabLookup = make(map[string]TabSettings)
)

// validDirectoryTabs indicates if a tab is enabled in the directory view.
var validDirectoryTabs = map[string]bool{
	"licenses":       true,
	"overview":       true,
	"subdirectories": true,
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

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForPackage(ctx context.Context, r *http.Request, tab string, ds internal.DataSource, pkg *internal.VersionedPackage) (interface{}, error) {
	switch tab {
	case "doc":
		return fetchDocumentationDetails(pkg), nil
	case "versions":
		return fetchPackageVersionsDetails(ctx, ds, pkg)
	case "subdirectories":
		return fetchDirectoryDetails(ctx, ds, pkg.Path, &pkg.ModuleInfo, pkg.Licenses, false)
	case "imports":
		return fetchImportsDetails(ctx, ds, pkg)
	case "importedby":
		return fetchImportedByDetails(ctx, ds, pkg)
	case "licenses":
		return fetchPackageLicensesDetails(ctx, ds, pkg)
	case "overview":
		return constructPackageOverviewDetails(pkg, urlIsVersioned(r.URL)), nil
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

func urlIsVersioned(url *url.URL) bool {
	return strings.ContainsRune(url.Path, '@')
}

// fetchDetailsForModule returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForModule(ctx context.Context, r *http.Request, tab string, ds internal.DataSource, mi *internal.ModuleInfo, licenses []*licenses.License) (interface{}, error) {
	switch tab {
	case "packages":
		return fetchDirectoryDetails(ctx, ds, mi.ModulePath, mi, licensesToMetadatas(licenses), true)
	case "licenses":
		return &LicensesDetails{Licenses: transformLicenses(mi.ModulePath, mi.Version, licenses)}, nil
	case "versions":
		return fetchModuleVersionsDetails(ctx, ds, mi)
	case "overview":
		// TODO(b/138448402): implement remaining module views.
		return constructOverviewDetails(mi, mi.IsRedistributable, urlIsVersioned(r.URL)), nil
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// constructDetailsForDirectory returns tab details by delegating to the correct
// detail handler.
func constructDetailsForDirectory(r *http.Request, tab string, dir *internal.Directory, licenses []*licenses.License) (interface{}, error) {
	switch tab {
	case "overview":
		return constructOverviewDetails(&dir.ModuleInfo, dir.ModuleInfo.IsRedistributable, urlIsVersioned(r.URL)), nil
	case "subdirectories":
		// Ideally we would just use fetchDirectoryDetails here so that it
		// follows the same code path as fetchDetailsForModule and
		// fetchDetailsForPackage. However, since we already have the directory
		// and licenses info, it doesn't make sense to call
		// postgres.GetDirectory again.
		return createDirectory(dir, licensesToMetadatas(licenses), false)
	case "licenses":
		return &LicensesDetails{Licenses: transformLicenses(dir.ModulePath, dir.Version, licenses)}, nil
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}
