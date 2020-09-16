// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

// UnitPage contains data needed to render the unit template.
type UnitPage struct {
	basePage
	Unit          *internal.Unit
	NestedModules []*internal.ModuleInfo
	Packages      []*Package

	// Breadcrumb contains data used to render breadcrumb UI elements.
	Breadcrumb breadcrumb

	// Title is the title of the page.
	Title string

	// CanonicalURLPath is the representation of the URL path for the details
	// page, after the requested version and module path have been resolved.
	// For example, if the latest version of /my.module/pkg is version v1.5.2,
	// the canonical url for that path would be /my.module@v1.5.2/pkg
	CanonicalURLPath string

	// Licenses contains license metadata used in the header.
	Licenses []LicenseMetadata

	// Elapsed time since this version was committed.
	LastCommitTime string

	// The version string formatted for display.
	DisplayVersion string

	// LinkVersion is version string suitable for links used to compute
	// latest badges.
	LinkVersion string

	// LatestURL is a url pointing to the latest version of a unit.
	LatestURL string

	// PageType is the type of page (pkg, cmd, dir, etc.).
	PageType string

	// CanShowDetails indicates whether details can be shown or must be
	// hidden due to issues like license restrictions.
	CanShowDetails bool

	// UnitContentName is the display name of the selected unit content template".
	UnitContentName string

	// Readme is the rendered readme HTML.
	Readme *safehtml.HTML

	// ExpandReadme is holds the expandable readme state.
	ExpandReadme bool
}

var (
	unitTabLookup = map[string]TabSettings{
		tabDetails: {
			DisplayName:  "Details",
			TemplateName: "unit_details.tmpl",
		},
		tabVersions: {
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "unit_versions.tmpl",
		},
		tabImports: {
			AlwaysShowDetails: true,
			DisplayName:       "Imports",
			TemplateName:      "unit_imports.tmpl",
		},
		tabImportedBy: {
			AlwaysShowDetails: true,
			DisplayName:       "Imported By",
			TemplateName:      "unit_importedby.tmpl",
		},
		tabLicenses: {
			DisplayName:  "Licenses",
			TemplateName: "unit_licenses.tmpl",
		},
	}
)

// serveUnitPage serves a unit page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveUnitPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
	ds internal.DataSource, um *internal.UnitMeta, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "serveUnitPage(ctx, w, r, ds, %v, %q)", um, requestedVersion)
	unit, err := ds.GetUnit(ctx, um, internal.AllFields)
	if err != nil {
		return err
	}

	nestedModules, err := ds.GetNestedModules(ctx, unit.ModulePath)
	if err != nil {
		return err
	}

	dir, err := createDirectory(unit.Path, moduleInfo(unit), unit.Subdirectories, nestedModules, unit.Licenses, false)
	if err != nil {
		return err
	}

	readme, err := readmeContent(ctx, unit)
	if err != nil {
		return err
	}

	tab := r.FormValue("tab")
	if tab == "" {
		// Default to details tab when there is no tab param.
		tab = tabDetails
	}
	tabSettings, ok := unitTabLookup[tab]
	if !ok {
		// Redirect to clean URL path when tab param is invalid.
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}

	title, pageType := pageInfo(unit)
	basePage := s.newBasePage(r, title)
	basePage.AllowWideContent = true
	canShowDetails := unit.IsRedistributable || tabSettings.AlwaysShowDetails
	_, expandReadme := r.URL.Query()["readme"]
	page := UnitPage{
		basePage:      basePage,
		Unit:          unit,
		Packages:      dir.Packages,
		NestedModules: nestedModules,
		Breadcrumb:    displayBreadcrumb(unit, requestedVersion),
		Title:         title,
		CanonicalURLPath: constructPackageURL(
			unit.Path,
			unit.ModulePath,
			requestedVersion,
		),
		Licenses:        transformLicenseMetadata(unit.Licenses),
		LastCommitTime:  elapsedTime(unit.CommitTime),
		DisplayVersion:  displayVersion(unit.Version, unit.ModulePath),
		LinkVersion:     linkVersion(unit.Version, unit.ModulePath),
		LatestURL:       constructPackageURL(unit.Path, unit.ModulePath, middleware.LatestMinorVersionPlaceholder),
		PageType:        pageType,
		CanShowDetails:  canShowDetails,
		UnitContentName: tabSettings.DisplayName,
		Readme:          readme,
		ExpandReadme:    expandReadme,
	}

	s.servePage(ctx, w, tabSettings.TemplateName, page)
	return nil
}

// moduleInfo extracts module info from a unit. This is a shim
// for functions ReadmeHTML and createDirectory that will be removed
// when we complete the switch to units.
func moduleInfo(unit *internal.Unit) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath:        unit.ModulePath,
		Version:           unit.Version,
		CommitTime:        unit.CommitTime,
		IsRedistributable: unit.IsRedistributable,
		SourceInfo:        unit.SourceInfo,
	}
}

// readmeContent renders the readme to html.
func readmeContent(ctx context.Context, unit *internal.Unit) (*safehtml.HTML, error) {
	if unit.IsRedistributable && unit.Readme != nil {
		mi := moduleInfo(unit)
		readme, err := ReadmeHTML(ctx, mi, unit.Readme)
		if err != nil {
			return nil, err
		}
		return &readme, nil
	}

	return nil, nil
}

// pageInfo determines the title and pageType for a given unit.
func pageInfo(unit *internal.Unit) (title string, pageType string) {
	if unit.Path == stdlib.ModulePath {
		return "Standard library", pageTypeStdLib
	}
	if unit.IsCommand() {
		return effectiveName(unit.Path, unit.Name), pageTypeCommand
	}
	if unit.IsPackage() {
		return unit.Name, pageTypePackage
	}
	return unit.Path, pageTypeDirectory
}

// displayBreadcrumbs appends additional breadcrumb links for display
// to those for the given unit.
func displayBreadcrumb(unit *internal.Unit, requestedVersion string) breadcrumb {
	bc := breadcrumbPath(unit.Path, unit.ModulePath, requestedVersion)
	if unit.ModulePath == stdlib.ModulePath && unit.Path != stdlib.ModulePath {
		bc.Links = append([]link{{Href: "/std", Body: "Standard Library"}}, bc.Links...)
	}
	bc.Links = append([]link{{Href: "/", Body: "Discover Packages"}}, bc.Links...)
	return bc
}
