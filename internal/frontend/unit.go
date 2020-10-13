// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"path"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

// UnitPage contains data needed to render the unit template.
type UnitPage struct {
	basePage
	// Unit is the unit for this page.
	Unit *internal.UnitMeta

	// Breadcrumb contains data used to render breadcrumb UI elements.
	Breadcrumb breadcrumb

	// Title is the title of the page.
	Title string

	// URLPath is the path suitable for links on the page.
	URLPath string

	// CanonicalURLPath is the representation of the URL path for the details
	// page, after the requested version and module path have been resolved.
	// For example, if the latest version of /my.module/pkg is version v1.5.2,
	// the canonical url for that path would be /my.module@v1.5.2/pkg
	CanonicalURLPath string

	// The version string formatted for display.
	DisplayVersion string

	// LinkVersion is version string suitable for links used to compute
	// latest badges.
	LinkVersion string

	// LatestURL is a url pointing to the latest version of a unit.
	LatestURL string

	// PageType is the type of page (pkg, cmd, dir, std, or mod).
	PageType string

	// PageLabels are the labels that will be displayed
	// for a given page.
	PageLabels []string

	// CanShowDetails indicates whether details can be shown or must be
	// hidden due to issues like license restrictions.
	CanShowDetails bool

	// UnitContentName is the display name of the selected unit content template".
	UnitContentName string

	// Tabs contains data to render the varioius tabs on each details page.
	Tabs []TabSettings

	// Settings contains settings for the selected tab.
	SelectedTab TabSettings

	// Details contains data specific to the type of page being rendered.
	Details interface{}
}

// serveUnitPage serves a unit page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveUnitPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
	ds internal.DataSource, um *internal.UnitMeta, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "serveUnitPage(ctx, w, r, ds, %v, %q)", um, requestedVersion)

	tab := r.FormValue("tab")
	if tab == "" {
		// Default to details tab when there is no tab param.
		tab = tabMain
	}
	tabSettings, ok := unitTabLookup[tab]
	if !ok {
		// Redirect to clean URL path when tab param is invalid.
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}

	title := pageTitle(um)
	basePage := s.newBasePage(r, title)
	basePage.AllowWideContent = true
	canShowDetails := um.IsRedistributable || tabSettings.AlwaysShowDetails
	page := UnitPage{
		basePage:    basePage,
		Unit:        um,
		Breadcrumb:  displayBreadcrumb(um, requestedVersion),
		Title:       title,
		Tabs:        unitTabs,
		SelectedTab: tabSettings,
		URLPath: constructPackageURL(
			um.Path,
			um.ModulePath,
			requestedVersion,
		),
		CanonicalURLPath: constructPackageURL(
			um.Path,
			um.ModulePath,
			linkVersion(um.Version, um.ModulePath),
		),
		DisplayVersion:  displayVersion(um.Version, um.ModulePath),
		LinkVersion:     linkVersion(um.Version, um.ModulePath),
		LatestURL:       constructPackageURL(um.Path, um.ModulePath, middleware.LatestMinorVersionPlaceholder),
		PageLabels:      pageLabels(um),
		PageType:        pageType(um),
		CanShowDetails:  canShowDetails,
		UnitContentName: tabSettings.DisplayName,
	}
	d, err := fetchDetailsForUnit(r, tab, ds, um)
	if err != nil {
		return err
	}
	page.Details = d
	s.servePage(ctx, w, tabSettings.TemplateName, page)
	return nil
}

const (
	pageTypeModule    = "module"
	pageTypeDirectory = "directory"
	pageTypePackage   = "package"
	pageTypeCommand   = "command"
	pageTypeModuleStd = "std"
	pageTypeStdlib    = "standard library"
)

// pageTitle determines the pageTitles for a given unit.
// See TestPageTitlesAndTypes for examples.
func pageTitle(um *internal.UnitMeta) string {
	switch {
	case um.Path == stdlib.ModulePath:
		return "Standard library"
	case um.IsCommand():
		return effectiveName(um.Path, um.Name)
	case um.IsPackage():
		return um.Name
	case um.IsModule():
		return path.Base(um.Path)
	default:
		return path.Base(um.Path) + "/"
	}
}

// pageType determines the pageType for a given unit.
func pageType(um *internal.UnitMeta) string {
	if um.Path == stdlib.ModulePath {
		return pageTypeModuleStd
	}
	if um.IsCommand() {
		return pageTypeCommand
	}
	if um.IsPackage() {
		return pageTypePackage
	}
	if um.IsModule() {
		return pageTypeModule
	}
	return pageTypeDirectory
}

// pageLabels determines the labels to display for a given unit.
// See TestPageTitlesAndTypes for examples.
func pageLabels(um *internal.UnitMeta) []string {
	var pageTypes []string
	if um.Path == stdlib.ModulePath {
		return nil
	}
	if um.IsCommand() {
		pageTypes = append(pageTypes, pageTypeCommand)
	} else if um.IsPackage() {
		pageTypes = append(pageTypes, pageTypePackage)
	}
	if um.IsModule() {
		pageTypes = append(pageTypes, pageTypeModule)
	}
	if !um.IsPackage() && !um.IsModule() {
		pageTypes = append(pageTypes, pageTypeDirectory)
	}
	if stdlib.Contains(um.Path) {
		pageTypes = append(pageTypes, pageTypeStdlib)
	}
	return pageTypes
}

// displayBreadcrumbs appends additional breadcrumb links for display
// to those for the given unit.
func displayBreadcrumb(um *internal.UnitMeta, requestedVersion string) breadcrumb {
	bc := breadcrumbPath(um.Path, um.ModulePath, requestedVersion)
	if um.ModulePath == stdlib.ModulePath && um.Path != stdlib.ModulePath {
		bc.Links = append([]link{{Href: "/std", Body: "Standard library"}}, bc.Links...)
	}
	bc.Links = append([]link{{Href: "/", Body: "Discover Packages"}}, bc.Links...)
	return bc
}
