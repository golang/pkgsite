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
			Name:         legacyTabDoc,
			DisplayName:  "Doc",
			TemplateName: "pkg_doc.tmpl",
		},
		{
			Name:              legacyTabOverview,
			AlwaysShowDetails: true,
			DisplayName:       "Overview",
			TemplateName:      "overview.tmpl",
		},
		{
			Name:              legacyTabSubdirectories,
			AlwaysShowDetails: true,
			DisplayName:       "Subdirectories",
			TemplateName:      "subdirectories.tmpl",
		},
		{
			Name:              legacyTabVersions,
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:              legacyTabImports,
			DisplayName:       "Imports",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_imports.tmpl",
		},
		{
			Name:              legacyTabImportedBy,
			DisplayName:       "Imported By",
			AlwaysShowDetails: true,
			TemplateName:      "pkg_importedby.tmpl",
		},
		{
			Name:         legacyTabLicenses,
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	packageTabLookup = make(map[string]TabSettings)

	directoryTabSettings = make([]TabSettings, len(packageTabSettings))
	directoryTabLookup   = make(map[string]TabSettings)

	moduleTabSettings = []TabSettings{
		{
			Name:              legacyTabOverview,
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
			Name:              legacyTabVersions,
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "versions.tmpl",
		},
		{
			Name:         legacyTabLicenses,
			DisplayName:  "Licenses",
			TemplateName: "licenses.tmpl",
		},
	}
	moduleTabLookup = make(map[string]TabSettings)
)

// validDirectoryTabs indicates if a tab is enabled in the directory view.
var validDirectoryTabs = map[string]bool{
	legacyTabLicenses:       true,
	legacyTabOverview:       true,
	legacyTabSubdirectories: true,
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
	legacyTabDoc            = "doc"
	legacyTabOverview       = "overview"
	legacyTabSubdirectories = "subdirectories"
	legacyTabVersions       = "versions"
	legacyTabImports        = "imports"
	legacyTabImportedBy     = "importedby"
	legacyTabLicenses       = "licenses"
)

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForPackage(r *http.Request, tab string, ds internal.DataSource, um *internal.UnitMeta) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case legacyTabDoc:
		return fetchDocumentationDetails(ctx, ds, um)
	case legacyTabOverview:
		return fetchPackageOverviewDetails(ctx, ds, um, urlIsVersioned(r.URL))
	case legacyTabVersions:
		return fetchVersionsDetails(ctx, ds, um.Path, um.ModulePath)
	case legacyTabImports:
		return fetchImportsDetails(ctx, ds, um.Path, um.ModulePath, um.Version)
	case legacyTabImportedBy:
		return fetchImportedByDetails(ctx, ds, um.Path, um.ModulePath)
	case legacyTabLicenses:
		return fetchLicensesDetails(ctx, ds, um)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

func urlIsVersioned(url *url.URL) bool {
	return strings.ContainsRune(url.Path, '@')
}
