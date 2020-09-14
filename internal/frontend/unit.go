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
	Unit *internal.Unit

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

	title, pageType := pageInfo(unit)
	basePage := s.newBasePage(r, title)
	basePage.AllowWideContent = true
	tab := r.FormValue("tab")
	tabSettings, ok := unitTabLookup[tab]
	if !ok {
		tabSettings = unitTabLookup[tabDetails]
	}
	canShowDetails := unit.IsRedistributable || tabSettings.AlwaysShowDetails
	readme, err := readmeContent(ctx, unit)
	if err != nil {
		return err
	}

	_, expandReadme := r.URL.Query()["readme"]
	page := UnitPage{
		basePage:   basePage,
		Unit:       unit,
		Breadcrumb: displayBreadcrumb(unit, requestedVersion),
		Title:      title,
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

func readmeContent(ctx context.Context, unit *internal.Unit) (*safehtml.HTML, error) {
	if unit.IsRedistributable && unit.Readme != nil {
		mi := &internal.ModuleInfo{
			ModulePath:        unit.ModulePath,
			Version:           unit.Version,
			CommitTime:        unit.CommitTime,
			IsRedistributable: unit.IsRedistributable,
			SourceInfo:        unit.SourceInfo,
		}
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
