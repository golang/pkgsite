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
	"golang.org/x/discovery/internal/stdlib"
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
	path, version, err := parsePathAndVersion(r.URL.Path, "pkg")
	if err != nil {
		log.Printf("handlePackageDetails: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}

	var pkg *internal.VersionedPackage
	code, epage := fetchPackageOrModule("pkg", path, version, func(ver string) error {
		var err error
		pkg, err = s.ds.GetPackage(r.Context(), path, ver)
		return err
	})
	if code == http.StatusNotFound {
		// We were not able to find the package at any version. In that case,
		// try and fetch the directory view.
		s.serveDirectoryPage(w, r, path, version)
		return
	}
	if code != http.StatusOK {
		s.serveErrorPage(w, r, code, epage)
		return
	}

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
			tab = "subdirectories"
		}
		settings = packageTabLookup[tab]
	}
	canShowDetails := pkg.IsRedistributable() || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(r.Context(), r, tab, s.ds, pkg)
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

// fetchPackageOrModule handles logic common to the initial phase of
// handling both packages and modules: fetching information about the package
// or module.
// It parses urlPath into an import path and version, then calls the get
// function with those values. If get fails because the version cannot be
// found, fetchPackageOrModule calls get again with the latest version,
// to see if any versions of the package/module exist, in order to provide a
// more helpful error message.
//
// fetchPackageOrModule returns the import path and version requested, an
// HTTP status code, and possibly an error page to display.
func fetchPackageOrModule(namespace, path, version string, get func(v string) error) (code int, _ *errorPage) {
	if version != internal.LatestVersion && !semver.IsValid(version) {
		// A valid semantic version was not requested.
		epage := &errorPage{Message: fmt.Sprintf("%q is not a valid semantic version.", version)}
		if namespace == "pkg" {
			epage.SecondaryMessage = suggestedSearch(path)
		}
		log.Printf("%s@%s: invalid version", path, version)
		return http.StatusBadRequest, epage
	}

	// Fetch the package or module from the database.
	err := get(version)
	if err == nil {
		// A package or module was found for this path and version.
		return http.StatusOK, nil
	}
	log.Printf("fetchPackageOrModule(%q, %q, %q): get error: %v",
		namespace, path, version, err)
	if !xerrors.Is(err, derrors.NotFound) {
		// Something went wrong in executing the get function.
		return http.StatusInternalServerError, nil
	}
	if version == internal.LatestVersion {
		// We were not able to find a module or package at any version.
		return http.StatusNotFound, nil
	}

	// We did not find the given version, but maybe there is another version
	// available for this package or module.
	if err := get(internal.LatestVersion); err != nil {
		log.Printf("error: get(%s, Latest) for %s: %v", path, namespace, err)
		// Couldn't get the latest version, for whatever reason. Treat
		// this like not finding the original version.
		return http.StatusNotFound, nil
	}

	// There is a later version of this package/module.
	word := "package"
	if namespace == "mod" {
		word = "module"
	}
	epage := &errorPage{
		Message: fmt.Sprintf("%s %s@%s is not available.", strings.ToTitle(word), path, version),
		SecondaryMessage: template.HTML(
			fmt.Sprintf(`There are other versions of this %s that are! To view them, <a href="/%s/%s?tab=versions">click here</a>.</p>`, word, namespace, path)),
	}
	return http.StatusSeeOther, epage
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

// moduleTitle constructs the details page title for pkg.
func moduleTitle(modulePath string) string {
	if stdlib.ModulePath == modulePath {
		return "Standard library"
	}
	return "Module " + modulePath
}

// handleModuleDetails applies database data to the appropriate template.
// Handles all endpoints that match "/mod/<module-path>[@<version>?tab=<tab>]".
func (s *Server) handleModuleDetails(w http.ResponseWriter, r *http.Request) {
	path, version, err := parsePathAndVersion(r.URL.Path, "mod")
	if err != nil {
		log.Printf("handleModuleDetails: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}

	ctx := r.Context()
	var moduleVersion *internal.VersionInfo
	code, epage := fetchPackageOrModule("mod", path, version, func(ver string) error {
		var err error
		moduleVersion, err = s.ds.GetVersionInfo(ctx, path, ver)
		return err
	})
	if code != http.StatusOK {
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
		basePage:       newBasePage(r, moduleTitle(moduleVersion.ModulePath)),
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

// parsePathAndVersion returns the path and version specified by urlPath.
// urlPath is assumed to be a valid path following the structure:
//   /<namespace>/<path>@<version>
// or
//  /<namespace/<path>
// where <namespace> is always pkg or mod.
// In the latter case, internal.LatestVersion is used for the version.
//
// Leading and trailing slashes in the urlPath are trimmed.
func parsePathAndVersion(urlPath, namespace string) (path, version string, err error) {
	defer derrors.Wrap(&err, "parsePathAndVersion(%q)", urlPath)

	if namespace != "mod" && namespace != "pkg" {
		return "", "", fmt.Errorf("invalid namespace: %q", namespace)
	}
	urlPath = strings.TrimPrefix(urlPath, "/"+namespace)
	parts := strings.Split(urlPath, "@")
	if len(parts) != 1 && len(parts) != 2 {
		return "", "", fmt.Errorf("malformed URL path %q", urlPath)
	}

	path = strings.TrimPrefix(parts[0], "/")
	if len(parts) == 1 {
		path = strings.TrimSuffix(path, "/")
	}

	// CheckPath checks that a module path is valid.
	if namespace == "mod" {
		if err := module.CheckImportPath(path); err != nil && path != stdlib.ModulePath {
			return "", "", fmt.Errorf("malformed module path %q: %v", path, err)
		}
	}
	if namespace == "pkg" {
		if err := module.CheckImportPath(path); err != nil {
			return "", "", fmt.Errorf("malformed import path %q: %v", path, err)
		}
	}
	if len(parts) == 1 {
		return path, internal.LatestVersion, nil
	}
	return path, strings.TrimRight(parts[1], "/"), nil
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

// fileSource returns the original filepath in the module zip where the given
// filePath can be found. For std, the corresponding URL in
// go.google.source.com/go is returned.
func fileSource(modulePath, version, filePath string) string {
	if modulePath != stdlib.ModulePath {
		return fmt.Sprintf("%s@%s/%s", modulePath, version, filePath)
	}

	root := strings.TrimPrefix(stdlib.GoRepoURL, "https://")
	tag, err := stdlib.TagForVersion(version)
	if err != nil {
		// This should never happen unless there is a bug in
		// stdlib.TagForVersion. In which case, fallback to the default
		// zipFilePath.
		log.Printf("fileSource: %v", err)
		return fmt.Sprintf("%s/+/refs/heads/master/%s", root, filePath)
	}
	return fmt.Sprintf("%s/+/refs/tags/%s/%s", root, tag, filePath)
}
