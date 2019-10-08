// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
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
}

var (
	packageTabSettings = []TabSettings{
		{
			Name:         "doc",
			DisplayName:  "Doc",
			TemplateName: "pkg_doc.tmpl",
		},
		{
			Name:         "readme",
			DisplayName:  "README",
			TemplateName: "readme.tmpl",
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

	moduleTabSettings = []TabSettings{
		{
			Name:         "readme",
			DisplayName:  "README",
			TemplateName: "readme.tmpl",
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

func init() {
	for _, d := range packageTabSettings {
		packageTabLookup[d.Name] = d
	}
	for _, d := range moduleTabSettings {
		moduleTabLookup[d.Name] = d
	}
}

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForPackage(ctx context.Context, r *http.Request, tab string, ds DataSource, pkg *internal.VersionedPackage) (interface{}, error) {
	switch tab {
	case "doc":
		return fetchDocumentationDetails(ctx, ds, pkg)
	case "versions":
		return fetchPackageVersionsDetails(ctx, ds, pkg)
	case "subdirectories":
		return fetchPackageDirectoryDetails(ctx, ds, pkg.Path, &pkg.VersionInfo)
	case "imports":
		return fetchImportsDetails(ctx, ds, pkg)
	case "importedby":
		return fetchImportedByDetails(ctx, ds, pkg)
	case "licenses":
		return fetchPackageLicensesDetails(ctx, ds, pkg)
	case "readme":
		return fetchReadMeDetails(ctx, ds, &pkg.VersionInfo)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// fetchDetailsForModule returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForModule(ctx context.Context, r *http.Request, tab string, ds DataSource, vi *internal.VersionInfo, licenses []*license.License) (interface{}, error) {
	switch tab {
	case "packages":
		return fetchModuleDirectoryDetails(ctx, ds, vi)
	case "licenses":
		return &LicensesDetails{Licenses: transformLicenses(vi.ModulePath, vi.Version, licenses)}, nil
	case "versions":
		return fetchModuleVersionsDetails(ctx, ds, vi)
	case "readme":
		// TODO(b/138448402): implement remaining module views.
		return fetchReadMeDetails(ctx, ds, vi)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}
