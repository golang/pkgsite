// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

// UnitPage contains data needed to render the unit template.
type UnitPage struct {
	basePage
	// Unit is the unit for this page.
	Unit *internal.Unit

	// NestedModules are nested modules relative to the path for the unit.
	NestedModules []*NestedModule

	// Subdirectories are packages in subdirectories relative to the path for
	// the unit.
	Subdirectories []*Subdirectory

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
	Readme safehtml.HTML

	// ExpandReadme is holds the expandable readme state.
	ExpandReadme bool

	// Tabs contains data to render the varioius tabs on each details page.
	Tabs []TabSettings

	// Settings contains settings for the selected tab.
	SelectedTab TabSettings

	// PackageDetails contains data used to render the
	// versions, licenses, imports, and importedby tabs.
	PackageDetails interface{}

	// ImportedByCount is the number of packages that import this path.
	// When the count is > limit it will read as 'limit+'. This field
	// is not supported when using a datasource proxy.
	ImportedByCount string

	DocBody       safehtml.HTML
	DocOutline    safehtml.HTML
	MobileOutline safehtml.HTML

	// SourceFiles contains .go files for the package.
	SourceFiles []*File
}

// File is a source file for a package.
type File struct {
	Name string
	URL  string
}

// NestedModule is a nested module relative to the path of a given unit.
// This content is used in the Directories section of the unit page.
type NestedModule struct {
	Suffix string // suffix after the unit path
	URL    string
}

// Subdirectory is a package in a subdirectory relative to the path of a given
// unit. This content is used in the Directories section of the unit page.
type Subdirectory struct {
	Suffix   string
	URL      string
	Synopsis string
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
	unit, err := ds.GetUnit(ctx, um, internal.AllFields)
	if err != nil {
		return err
	}

	// importedByCount is not supported when using a datasource proxy.
	importedByCount := "0"
	db, ok := ds.(*postgres.DB)
	if ok {
		importedBy, err := db.GetImportedBy(ctx, unit.Path, unit.ModulePath, importedByLimit)
		if err != nil {
			return err
		}
		// If we reached the query limit, then we don't know the total
		// and we'll indicate that with a '+'. For example, if the limit
		// is 101 and we get 101 results, then we'll show '100+ Imported by'.
		importedByCount = strconv.Itoa(len(importedBy))
		if len(importedBy) == importedByLimit {
			importedByCount = strconv.Itoa(len(importedBy)-1) + "+"
		}
	}

	nestedModules, err := getNestedModules(ctx, ds, &unit.UnitMeta)
	if err != nil {
		return err
	}
	subdirectories := getSubdirectories(&unit.UnitMeta, unit.Subdirectories)
	if err != nil {
		return err
	}

	readme, err := readmeContent(ctx, unit)
	if err != nil {
		return err
	}

	var (
		docBody, docOutline, mobileOutline safehtml.HTML
		files                              []*File
	)
	if unit.Documentation != nil {
		docHTML := getHTML(ctx, unit)
		// TODO: Deprecate godoc.Parse. The sidenav and body can
		// either be rendered using separate functions, or all this content can
		// be passed to the template via the UnitPage struct.
		b, err := godoc.Parse(docHTML, godoc.BodySection)
		if err != nil {
			return err
		}
		docBody = b
		o, err := godoc.Parse(docHTML, godoc.SidenavSection)
		if err != nil {
			return err
		}
		docOutline = o
		m, err := godoc.Parse(docHTML, godoc.SidenavMobileSection)
		if err != nil {
			return err
		}
		mobileOutline = m

		files, err = sourceFiles(unit)
		if err != nil {
			return err
		}
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
		basePage:       basePage,
		Unit:           unit,
		Subdirectories: subdirectories,
		NestedModules:  nestedModules,
		Breadcrumb:     displayBreadcrumb(unit, requestedVersion),
		Title:          title,
		Tabs:           unitTabs,
		SelectedTab:    tabSettings,
		URLPath: constructPackageURL(
			unit.Path,
			unit.ModulePath,
			requestedVersion,
		),
		CanonicalURLPath: constructPackageURL(
			unit.Path,
			unit.ModulePath,
			linkVersion(unit.Version, unit.ModulePath),
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
		DocOutline:      docOutline,
		DocBody:         docBody,
		SourceFiles:     files,
		MobileOutline:   mobileOutline,
		ImportedByCount: importedByCount,
	}

	if tab != tabDetails {
		packageDetails, err := fetchDetailsForPackage(r, tab, ds, &unit.UnitMeta)
		if err != nil {
			return err
		}
		page.PackageDetails = packageDetails
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
func readmeContent(ctx context.Context, unit *internal.Unit) (safehtml.HTML, error) {
	if unit.IsRedistributable && unit.Readme != nil {
		mi := moduleInfo(unit)
		readme, err := ReadmeHTML(ctx, mi, unit.Readme)
		if err != nil {
			return safehtml.HTML{}, err
		}
		return readme, nil
	}
	return safehtml.HTML{}, nil
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
	if unit.IsModule() {
		return path.Base(unit.Path), pageTypeModule
	}
	return path.Base(unit.Path) + "/", pageTypeDirectory
}

// displayBreadcrumbs appends additional breadcrumb links for display
// to those for the given unit.
func displayBreadcrumb(unit *internal.Unit, requestedVersion string) breadcrumb {
	bc := breadcrumbPath(unit.Path, unit.ModulePath, requestedVersion)
	if unit.ModulePath == stdlib.ModulePath && unit.Path != stdlib.ModulePath {
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
