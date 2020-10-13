// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
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

var (
	unitTabs = []TabSettings{
		{
			Name:         tabDetails,
			DisplayName:  "Main",
			TemplateName: "unit_details.tmpl",
		},
		{
			Name:              tabVersions,
			AlwaysShowDetails: true,
			DisplayName:       "Versions",
			TemplateName:      "unit_versions.tmpl",
		},
		{
			Name:              tabImports,
			AlwaysShowDetails: true,
			DisplayName:       "Imports",
			TemplateName:      "unit_imports.tmpl",
		},
		{
			Name:              tabImportedBy,
			AlwaysShowDetails: true,
			DisplayName:       "Imported By",
			TemplateName:      "unit_importedby.tmpl",
		},
		{
			Name:         tabLicenses,
			DisplayName:  "Licenses",
			TemplateName: "unit_licenses.tmpl",
		},
	}
	unitTabLookup = make(map[string]TabSettings, len(unitTabs))
)

func init() {
	for _, t := range unitTabs {
		unitTabLookup[t.Name] = t
	}
}

// serveUnitPage serves a unit page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveUnitPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
	ds internal.DataSource, um *internal.UnitMeta, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "serveUnitPage(ctx, w, r, ds, %v, %q)", um, requestedVersion)

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
	if tab == tabDetails {
		_, expandReadme := r.URL.Query()["readme"]
		d, err := fetchMainDetails(ctx, ds, um)
		if err != nil {
			return err
		}
		d.ExpandReadme = expandReadme
		page.Details = d
	}
	if tab != tabDetails {
		d, err := fetchDetailsForPackage(r, tab, ds, um)
		if err != nil {
			return err
		}
		page.Details = d
	}
	s.servePage(ctx, w, tabSettings.TemplateName, page)
	return nil
}

func getHTML(ctx context.Context, u *internal.Unit) safehtml.HTML {
	if experiment.IsActive(ctx, internal.ExperimentFrontendRenderDoc) && len(u.Documentation.Source) > 0 {
		dd, err := renderDoc(ctx, u)
		if err != nil {
			log.Errorf(ctx, "render doc failed: %v", err)
			// Fall through to use stored doc.
		} else {
			return dd.Documentation
		}
	}
	return u.Documentation.HTML
}

// moduleInfo extracts module info from a unit. This is a shim
// for functions ReadmeHTML and createDirectory that will be removed
// when we complete the switch to units.
func moduleInfo(um *internal.UnitMeta) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
		SourceInfo:        um.SourceInfo,
	}
}

// readmeContent renders the readme to html.
func readmeContent(ctx context.Context, um *internal.UnitMeta, readme *internal.Readme) (safehtml.HTML, error) {
	if um.IsRedistributable && readme != nil {
		mi := moduleInfo(um)
		readme, err := ReadmeHTML(ctx, mi, readme)
		if err != nil {
			return safehtml.HTML{}, err
		}
		return readme, nil
	}
	return safehtml.HTML{}, nil
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

func getNestedModules(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) ([]*NestedModule, error) {
	nestedModules, err := ds.GetNestedModules(ctx, um.ModulePath)
	if err != nil {
		return nil, err
	}
	var mods []*NestedModule
	for _, m := range nestedModules {
		if m.SeriesPath() == internal.SeriesPathForModule(um.ModulePath) {
			continue
		}
		if !strings.HasPrefix(m.ModulePath, um.Path+"/") {
			continue
		}
		mods = append(mods, &NestedModule{
			URL:    constructPackageURL(m.ModulePath, m.ModulePath, internal.LatestVersion),
			Suffix: internal.Suffix(m.SeriesPath(), um.Path),
		})
	}
	return mods, nil
}

func getSubdirectories(um *internal.UnitMeta, pkgs []*internal.PackageMeta) []*Subdirectory {
	var sdirs []*Subdirectory
	for _, pm := range pkgs {
		if um.Path == pm.Path {
			continue
		}
		sdirs = append(sdirs, &Subdirectory{
			URL:      constructPackageURL(pm.Path, um.ModulePath, linkVersion(um.Version, um.ModulePath)),
			Suffix:   internal.Suffix(pm.Path, um.Path),
			Synopsis: pm.Synopsis,
		})
	}
	sort.Slice(sdirs, func(i, j int) bool { return sdirs[i].Suffix < sdirs[j].Suffix })
	return sdirs
}
