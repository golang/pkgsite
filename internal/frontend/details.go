// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/xerrors"
)

// DetailsPage contains data for a package of module details template.
type DetailsPage struct {
	basePage
	CanShowDetails bool
	Settings       TabSettings
	Details        interface{}
	Header         interface{}
	Tabs           []TabSettings
	Namespace      string
}

// handlePackageDetails applies database data to the appropriate template.
// Handles all endpoints that match "/pkg/<import-path>[@<version>?tab=<tab>]".
func (s *Server) handlePackageDetails(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/pkg")
	path, version, err := parseImportPathAndVersion(urlPath)
	if err != nil {
		log.Print(err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	if version != internal.LatestVersion && !semver.IsValid(version) {
		log.Printf("%s@%s: invalid version", path, version)
		s.serveErrorPage(w, r, http.StatusBadRequest, &errorPage{
			Message:          fmt.Sprintf("%q is not a valid semantic version.", version),
			SecondaryMessage: suggestedSearch(path),
		})
		return
	}

	var (
		pkg *internal.VersionedPackage
		ctx = r.Context()
	)
	pkg, err = s.ds.GetPackage(ctx, path, version)
	if err != nil && !xerrors.Is(err, derrors.NotFound) {
		log.Print(err)
	}
	if err != nil {
		if !xerrors.Is(err, derrors.NotFound) {
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
		if version == internal.LatestVersion {
			// If the version is empty, it means that we already
			// tried fetching the latest version of the package,
			// and this package does not exist.
			//
			// In that case, we attempt to fetch a directory view
			// for this path.
			s.serveDirectoryPage(w, r, path, version)
			return
		}
		// Get the latest package to check if any versions of
		// the package exists.
		_, latestErr := s.ds.GetPackage(ctx, path, internal.LatestVersion)
		if latestErr == nil {
			s.serveErrorPage(w, r, http.StatusNotFound, &errorPage{
				Message: fmt.Sprintf("Package %s@%s is not available.", path, version),
				SecondaryMessage: template.HTML(
					fmt.Sprintf(`There are other versions of this package that are! To view them, <a href="/pkg/%s?tab=versions">click here</a>.</p>`, path)),
			})
			return
		}
		if !xerrors.Is(err, derrors.NotFound) {
			// GetPackage at version returned a NotFound error, but
			// GetPackage at latest returned a different error.
			log.Printf("error getting latest package for %s: %v", path, latestErr)
		}
		s.serveDirectoryPage(w, r, path, version)
		return
	}

	version = pkg.VersionInfo.Version
	pkgHeader, err := createPackage(&pkg.Package, &pkg.VersionInfo)
	if err != nil {
		log.Printf("error creating package header for %s@%s: %v", path, version, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	tab := r.FormValue("tab")
	settings, ok := packageTabLookup[tab]
	if !ok {
		if pkg.IsRedistributable() {
			tab = "doc"
		} else {
			tab = "module"
		}
		settings = packageTabLookup[tab]
	}
	canShowDetails := pkg.IsRedistributable() || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(ctx, r, tab, s.ds, pkg)
		if err != nil {
			log.Printf("error fetching page for %q: %v", tab, err)
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
	}

	page := &DetailsPage{
		basePage:       newBasePage(r, packageTitle(&pkg.Package)),
		Settings:       settings,
		Header:         pkgHeader,
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           packageTabSettings,
		Namespace:      "pkg",
	}
	s.servePage(w, settings.TemplateName, page)
}

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForPackage(ctx context.Context, r *http.Request, tab string, ds DataSource, pkg *internal.VersionedPackage) (interface{}, error) {
	switch tab {
	case "doc":
		return fetchDocumentationDetails(ctx, ds, pkg)
	case "versions":
		return fetchPackageVersionsDetails(ctx, ds, pkg)
	case "module":
		return fetchModuleDetails(ctx, ds, &pkg.VersionInfo)
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

// handleModuleDetails applies database data to the appropriate template.
// Handles all endpoints that match "/mod/<module-path>[@<version>?tab=<tab>]".
func (s *Server) handleModuleDetails(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/mod")
	path, version, err := parseImportPathAndVersion(urlPath)
	if err != nil {
		log.Print(err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	if version != internal.LatestVersion && !semver.IsValid(version) {
		s.serveErrorPage(w, r, http.StatusBadRequest, &errorPage{
			Message: fmt.Sprintf("%q is not a valid semantic version.", version),
		})
		return
	}

	ctx := r.Context()
	var moduleVersion *internal.VersionInfo
	moduleVersion, err = s.ds.GetVersionInfo(ctx, path, version)
	if err != nil {
		code := http.StatusNotFound
		if !xerrors.Is(err, derrors.NotFound) {
			log.Print(err)
			code = http.StatusInternalServerError
		}
		var epage *errorPage
		if version != internal.LatestVersion {
			// The specific requested version doesn't exist.
			// See if any versions do by getting the latest package.
			_, latestErr := s.ds.GetVersionInfo(ctx, path, internal.LatestVersion)
			if latestErr == nil {
				epage = &errorPage{
					Message: fmt.Sprintf("Module %s@%s is not available.", path, version),
					SecondaryMessage: template.HTML(
						fmt.Sprintf(`There are other versions of this module that are! To view them, <a href="/mod/%s?tab=versions">click here</a>.</p>`, path)),
				}
			} else if xerrors.Is(err, derrors.NotFound) && !xerrors.Is(latestErr, derrors.NotFound) {
				// GetVersionInfo returned NotFound at version but GetVersionInfo at
				// latest did not.
				log.Printf("error getting latest module for %s: %v", path, latestErr)
			}
		}
		s.serveErrorPage(w, r, code, epage)
		return
	}
	// Here, moduleVersion is a valid *VersionInfo.
	licenses, err := s.ds.GetModuleLicenses(ctx, moduleVersion.ModulePath, moduleVersion.Version)
	if err != nil {
		log.Printf("error getting module licenses for %s@%s: %v", moduleVersion.ModulePath, moduleVersion.Version, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	tab := r.FormValue("tab")
	settings, ok := moduleTabLookup[tab]
	if !ok {
		tab = "readme"
		settings = moduleTabLookup["readme"]
	}

	modHeader := createModule(moduleVersion, license.ToMetadatas(licenses))
	canShowDetails := modHeader.IsRedistributable || settings.AlwaysShowDetails
	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForModule(ctx, r, tab, s.ds, moduleVersion, licenses)
		if err != nil {
			log.Printf("error fetching page for %q: %v", tab, err)
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
	}
	page := &DetailsPage{
		basePage:       newBasePage(r, moduleVersion.ModulePath),
		Settings:       settings,
		Header:         modHeader,
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		Namespace:      "mod",
	}
	s.servePage(w, settings.TemplateName, page)
}

// fetchDetailsForModule returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForModule(ctx context.Context, r *http.Request, tab string, ds DataSource, vi *internal.VersionInfo, licenses []*license.License) (interface{}, error) {
	switch tab {
	case "packages":
		return fetchModuleDetails(ctx, ds, vi)
	case "licenses":
		return &LicensesDetails{Licenses: transformLicenses(licenses)}, nil
	case "versions":
		return fetchModuleVersionsDetails(ctx, ds, vi)
	case "readme":
		// TODO(b/138448402): implement remaining module views.
		return fetchReadMeDetails(ctx, ds, vi)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// parseImportPathAndVersion returns the import path and version specified by
// urlPath. urlPath is assumed to be a valid path following the structures
//   /<path>@<version>
// or
//   /<path>
// In the latter case, internal.LatestVersion is used for the version.
//
// Leading and trailing slashes in the import path are trimmed.
func parseImportPathAndVersion(urlPath string) (importPath, version string, err error) {
	defer derrors.Wrap(&err, "parseImportPathAndVersion(%q)", urlPath)

	parts := strings.Split(urlPath, "@")
	if len(parts) != 1 && len(parts) != 2 {
		return "", "", fmt.Errorf("malformed URL path %q", urlPath)
	}

	importPath = strings.TrimPrefix(parts[0], "/")
	if len(parts) == 1 {
		importPath = strings.TrimSuffix(importPath, "/")
	}
	if err := module.CheckImportPath(importPath); err != nil {
		return "", "", fmt.Errorf("malformed import path %q: %v", importPath, err)
	}

	if len(parts) == 1 {
		return importPath, internal.LatestVersion, nil
	}
	return importPath, strings.TrimRight(parts[1], "/"), nil
}

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
			Name:              "module",
			AlwaysShowDetails: true,
			DisplayName:       "Module",
			TemplateName:      "module.tmpl",
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
			TemplateName:      "module.tmpl",
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

// Package contains information for an individual package.
type Package struct {
	Module
	Path              string
	Suffix            string
	Synopsis          string
	IsRedistributable bool
	Licenses          []LicenseMetadata
}

// Module contains information for an individual module.
type Module struct {
	Version           string
	Path              string
	CommitTime        string
	RepositoryURL     string
	IsRedistributable bool
	Licenses          []LicenseMetadata
}

// createPackage returns a *Package based on the fields of the specified
// internal package and version info.
func createPackage(pkg *internal.Package, vi *internal.VersionInfo) (_ *Package, err error) {
	defer derrors.Wrap(&err, "createPackage(%v, %v)", pkg, vi)

	if pkg == nil || vi == nil {
		return nil, fmt.Errorf("package and version info must not be nil")
	}

	suffix := strings.TrimPrefix(strings.TrimPrefix(pkg.Path, vi.ModulePath), "/")
	if suffix == "" {
		suffix = effectiveName(pkg) + " (root)"
	}

	var modLicenses []*license.Metadata
	for _, lm := range pkg.Licenses {
		if path.Dir(lm.FilePath) == "." {
			modLicenses = append(modLicenses, lm)
		}
	}

	m := createModule(vi, modLicenses)
	return &Package{
		Path:              pkg.Path,
		Suffix:            suffix,
		Synopsis:          pkg.Synopsis,
		IsRedistributable: pkg.IsRedistributable(),
		Licenses:          transformLicenseMetadata(pkg.Licenses),
		Module:            *m,
	}, nil
}

// createModule returns a *Module based on the fields of the specified
// versionInfo.
func createModule(vi *internal.VersionInfo, licmetas []*license.Metadata) *Module {
	return &Module{
		Version:           vi.Version,
		Path:              vi.ModulePath,
		CommitTime:        elapsedTime(vi.CommitTime),
		RepositoryURL:     vi.RepositoryURL,
		IsRedistributable: license.AreRedistributable(licmetas),
		Licenses:          transformLicenseMetadata(licmetas),
	}
}

// inStdLib reports whether the package is part of the Go standard library.
func inStdLib(path string) bool {
	if i := strings.IndexByte(path, '/'); i != -1 {
		return !strings.Contains(path[:i], ".")
	}
	return !strings.Contains(path, ".")
}

// effectiveName returns either the command name or package name.
func effectiveName(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return pkg.Name
	}
	var prefix string
	if pkg.Path[len(pkg.Path)-3:] == "/v1" {
		prefix = pkg.Path[:len(pkg.Path)-3]
	} else {
		prefix, _, _ = module.SplitPathVersion(pkg.Path)
	}
	_, base := path.Split(prefix)
	return base
}

// packageTitle constructs the details page title for pkg.
func packageTitle(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return "Package " + pkg.Name
	}
	return "Command " + effectiveName(pkg)
}

// elapsedTime takes a date and returns returns human-readable,
// relative timestamps based on the following rules:
// (1) 'X hours ago' when X < 6
// (2) 'today' between 6 hours and 1 day ago
// (3) 'Y days ago' when Y < 6
// (4) A date formatted like "Jan 2, 2006" for anything further back
func elapsedTime(date time.Time) string {
	elapsedHours := int(time.Since(date).Hours())
	if elapsedHours == 1 {
		return "1 hour ago"
	} else if elapsedHours < 6 {
		return fmt.Sprintf("%d hours ago", elapsedHours)
	}

	elapsedDays := elapsedHours / 24
	if elapsedDays < 1 {
		return "today"
	} else if elapsedDays == 1 {
		return "1 day ago"
	} else if elapsedDays < 6 {
		return fmt.Sprintf("%d days ago", elapsedDays)
	}

	return date.Format("Jan _2, 2006")
}

// DocumentationDetails contains data for the doc template.
type DocumentationDetails struct {
	ModulePath    string
	Documentation template.HTML
}

// fetchDocumentationDetails fetches data for the package specified by path and version
// from the database and returns a DocumentationDetails.
func fetchDocumentationDetails(ctx context.Context, ds DataSource, pkg *internal.VersionedPackage) (*DocumentationDetails, error) {
	return &DocumentationDetails{
		ModulePath:    pkg.VersionInfo.ModulePath,
		Documentation: template.HTML(pkg.DocumentationHTML),
	}, nil
}

// ModuleDetails contains all of the data that the module template
// needs to populate.
type ModuleDetails struct {
	ModulePath string
	Version    string
	Packages   []*Package
}

// fetchModuleDetails fetches data for the module version specified by pkgPath and pkgversion
// from the database and returns a ModuleDetails.
func fetchModuleDetails(ctx context.Context, ds DataSource, vi *internal.VersionInfo) (*ModuleDetails, error) {
	dbPackages, err := ds.GetPackagesInVersion(ctx, vi.ModulePath, vi.Version)
	if err != nil {
		return nil, err
	}

	var packages []*Package
	for _, p := range dbPackages {
		newPkg, err := createPackage(p, vi)
		if err != nil {
			return nil, err
		}
		if p.IsRedistributable() {
			newPkg.Synopsis = p.Synopsis
		}
		packages = append(packages, newPkg)
	}

	return &ModuleDetails{
		ModulePath: vi.ModulePath,
		Version:    vi.Version,
		Packages:   packages,
	}, nil
}
