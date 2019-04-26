// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// DetailsPage contains data for the doc template.
type DetailsPage struct {
	CanShowDetails bool
	Details        interface{}
	PackageHeader  *Package
}

// OverviewDetails contains all of the data that the overview template
// needs to populate.
type OverviewDetails struct {
	ModulePath string
	ReadMe     template.HTML
}

// DocumentationDetails contains data for the doc template.
type DocumentationDetails struct {
	ModulePath string
}

// ModuleDetails contains all of the data that the module template
// needs to populate.
type ModuleDetails struct {
	ModulePath string
	Version    string
	ReadMe     template.HTML
	Packages   []*Package
}

// ImportsDetails contains information for a package's imports.
type ImportsDetails struct {
	// Imports is an array of packages representing the package's imports
	// that are not in the Go standard library.
	Imports []*internal.Import

	// StdLib is an array of packages representing the package's imports
	// that are in the Go standard library.
	StdLib []*internal.Import
}

// ImportedByDetails contains information for the collection of packages that
// import a given package.
type ImportedByDetails struct {
	// ImportedBy is the collection of packages that import the given
	// package.
	ImportedBy []*internal.Import
}

// License contains information used for a single license section.
type License struct {
	*internal.License
	Anchor string
}

// LicensesDetails contains license information for a package.
type LicensesDetails struct {
	Licenses []License
}

// LicenseInfo contains license metadata that is used in the package header.
type LicenseInfo struct {
	*internal.LicenseInfo
	Anchor string
}

// VersionsDetails contains all the data that the versions tab
// template needs to populate.
type VersionsDetails struct {
	Versions []*MajorVersionGroup
}

// MajorVersionGroup represents the major level of the versions
// list hierarchy (i.e. "v1").
type MajorVersionGroup struct {
	Level    string
	Latest   *Package
	Versions []*MinorVersionGroup
}

// MinorVersionGroup represents the major/minor level of the versions
// list hierarchy (i.e. "1.5").
type MinorVersionGroup struct {
	Level    string
	Latest   *Package
	Versions []*Package
}

// Package contains information for an individual package.
type Package struct {
	Name              string
	Version           string
	Path              string
	ModulePath        string
	Synopsis          string
	CommitTime        string
	Title             string
	Suffix            string
	Licenses          []LicenseInfo
	IsCommand         bool
	IsRedistributable bool
}

// transformLicenseInfos transforms an internal.LicenseInfo into a LicenseInfo,
// by adding an anchor field.
func transformLicenseInfos(dbLicenses []*internal.LicenseInfo) []LicenseInfo {
	var licenseInfos []LicenseInfo
	for _, l := range dbLicenses {
		licenseInfos = append(licenseInfos, LicenseInfo{
			LicenseInfo: l,
			Anchor:      licenseAnchor(l.FilePath),
		})
	}
	return licenseInfos
}

// createPackageHeader returns a *Package based on the fields of the specified
// package. It assumes that pkg and pkg.Version are not nil.
func createPackageHeader(pkg *internal.VersionedPackage) (*Package, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	var isCmd bool
	if pkg.Name == "main" {
		isCmd = true
	}
	name := packageName(pkg.Name, pkg.Path)
	return &Package{
		Name:              name,
		IsCommand:         isCmd,
		Title:             packageTitle(name, isCmd),
		Version:           pkg.VersionInfo.Version,
		Path:              pkg.Path,
		Synopsis:          pkg.Package.Synopsis,
		Suffix:            pkg.Package.Suffix,
		Licenses:          transformLicenseInfos(pkg.Licenses),
		CommitTime:        elapsedTime(pkg.VersionInfo.CommitTime),
		IsRedistributable: pkg.IsRedistributable(),
	}, nil
}

// inStdLib reports whether the package is part of the Go standard library.
func inStdLib(path string) bool {
	if i := strings.IndexByte(path, '/'); i != -1 {
		return !strings.Contains(path[:i], ".")
	}
	return !strings.Contains(path, ".")
}

// packageName returns name if it is not "main". Otherwise, it returns the last
// element of pkg.Path that is not a version identifier (such as "v2"). For
// example, if name is "main" and path is foo/bar/v2, name will be "bar".
func packageName(name, path string) string {
	if name != "main" {
		return name
	}

	if path[len(path)-3:] == "/v1" {
		return filepath.Base(path[:len(path)-3])
	}

	prefix, _, _ := module.SplitPathVersion(path)
	return filepath.Base(prefix)
}

// packageTitle returns name prefixed by "Command" if isCommand is true and
// "Package" if false.
func packageTitle(name string, isCommand bool) string {
	if isCommand {
		return fmt.Sprintf("Command %s", name)
	}
	return fmt.Sprintf("Package %s", name)
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

// fetchOverviewDetails fetches data for the module version specified by path and version
// from the database and returns a OverviewDetails.
func fetchOverviewDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*OverviewDetails, error) {
	return &OverviewDetails{
		ModulePath: pkg.VersionInfo.ModulePath,
		ReadMe:     readmeHTML(pkg.VersionInfo.ReadmeFilePath, pkg.VersionInfo.ReadmeContents),
	}, nil
}

// fetchDocumentationDetails fetches data for the package specified by path and version
// from the database and returns a DocumentationDetails.
func fetchDocumentationDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*DocumentationDetails, error) {
	return &DocumentationDetails{pkg.VersionInfo.ModulePath}, nil
}

// fetchModuleDetails fetches data for the module version specified by pkgPath and pkgversion
// from the database and returns a ModuleDetails.
func fetchModuleDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*ModuleDetails, error) {
	version, err := db.GetVersionForPackage(ctx, pkg.Path, pkg.VersionInfo.Version)
	if err != nil {
		return nil, fmt.Errorf("db.GetVersionForPackage(ctx, %q, %q): %v", pkg.Path, pkg.VersionInfo.Version, err)
	}

	var packages []*Package
	for _, p := range version.Packages {
		if p.Suffix == "" {
			// Display the package name if the package is at the
			// root of the module.
			p.Suffix = p.Name
		}
		packages = append(packages, &Package{
			Name:       p.Name,
			Path:       p.Path,
			Synopsis:   p.Synopsis,
			Licenses:   transformLicenseInfos(p.Licenses),
			Version:    version.Version,
			ModulePath: version.ModulePath,
			Suffix:     p.Suffix,
		})
	}

	return &ModuleDetails{
		ModulePath: version.ModulePath,
		Version:    pkg.VersionInfo.Version,
		ReadMe:     readmeHTML(version.ReadmeFilePath, version.ReadmeContents),
		Packages:   packages,
	}, nil
}

// fetchVersionsDetails fetches data for the module version specified by path and version
// from the database and returns a VersionsDetails.
func fetchVersionsDetails(ctx context.Context, db *postgres.DB, pkg *internal.Package) (*VersionsDetails, error) {
	versions, err := db.GetTaggedVersionsForPackageSeries(ctx, pkg.Path)
	if err != nil {
		return nil, fmt.Errorf("db.GetTaggedVersions(%q): %v", pkg.Path, err)
	}

	// If no tagged versions for the package series are found,
	// fetch the pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = db.GetPseudoVersionsForPackageSeries(ctx, pkg.Path)
		if err != nil {
			return nil, fmt.Errorf("db.GetPseudoVersions(%q): %v", pkg.Path, err)
		}
	}

	var (
		mvg             = []*MajorVersionGroup{}
		prevMajor       = ""
		prevMajMin      = ""
		prevMajorIndex  = -1
		prevMajMinIndex = -1
	)

	for _, v := range versions {
		vStr := v.Version
		major := semver.Major(vStr)
		majMin := strings.TrimPrefix(semver.MajorMinor(vStr), "v")
		fullVersion := strings.TrimPrefix(vStr, "v")
		// It is a bit overly defensive to accept all conventions for leading and
		// trailing slashes here.
		pkgPath := strings.TrimSuffix(v.ModulePath, "/") + "/" + strings.TrimPrefix(pkg.Suffix, "/")

		if prevMajor != major {
			prevMajorIndex = len(mvg)
			prevMajor = major
			mvg = append(mvg, &MajorVersionGroup{
				Level: major,
				Latest: &Package{
					Version:    fullVersion,
					Path:       pkgPath,
					CommitTime: elapsedTime(v.CommitTime),
				},
				Versions: []*MinorVersionGroup{},
			})
		}

		if prevMajMin != majMin {
			prevMajMinIndex = len(mvg[prevMajorIndex].Versions)
			prevMajMin = majMin
			mvg[prevMajorIndex].Versions = append(mvg[prevMajorIndex].Versions, &MinorVersionGroup{
				Level: majMin,
				Latest: &Package{
					Version:    fullVersion,
					Path:       pkgPath,
					CommitTime: elapsedTime(v.CommitTime),
				},
			})
		}

		mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions = append(mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions, &Package{
			Version:    fullVersion,
			Path:       pkgPath,
			CommitTime: elapsedTime(v.CommitTime),
		})
	}

	return &VersionsDetails{
		Versions: mvg,
	}, nil
}

// licenseAnchor returns the anchor that should be used to jump to the specific
// license on the licenses page.
func licenseAnchor(filePath string) string {
	return url.QueryEscape(filePath)
}

// fetchLicensesDetails fetches license data for the package version specified by
// path and version from the database and returns a LicensesDetails.
func fetchLicensesDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*LicensesDetails, error) {
	dbLicenses, err := db.GetLicenses(ctx, pkg.Path, pkg.VersionInfo.Version)
	if err != nil {
		return nil, fmt.Errorf("db.GetLicenses(ctx, %q, %q): %v", pkg.Path, pkg.VersionInfo.Version, err)
	}

	licenses := make([]License, len(dbLicenses))
	for i, l := range dbLicenses {
		licenses[i] = License{
			Anchor:  licenseAnchor(l.FilePath),
			License: l,
		}
	}

	return &LicensesDetails{
		Licenses: licenses,
	}, nil
}

// fetchImportsDetails fetches imports for the package version specified by
// path and version from the database and returns a ImportsDetails.
func fetchImportsDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*ImportsDetails, error) {
	dbImports, err := db.GetImports(ctx, pkg.Path, pkg.VersionInfo.Version)
	if err != nil {
		return nil, fmt.Errorf("db.GetImports(ctx, %q, %q): %v", pkg.Path, pkg.VersionInfo.Version, err)
	}

	var imports []*internal.Import
	var std []*internal.Import
	for _, p := range dbImports {
		if inStdLib(p.Path) {
			std = append(std, p)
		} else {
			imports = append(imports, p)
		}
	}

	return &ImportsDetails{
		Imports: imports,
		StdLib:  std,
	}, nil
}

// fetchImportedByDetails fetches importers for the package version specified by
// path and version from the database and returns a ImportedByDetails.
func fetchImportedByDetails(ctx context.Context, db *postgres.DB, pkg *internal.Package) (*ImportedByDetails, error) {
	importedByPaths, err := db.GetImportedBy(ctx, pkg.Path)
	if err != nil {
		return nil, fmt.Errorf("db.GetImportedBy(ctx, %q): %v", pkg.Path, err)
	}
	var importedBy []*internal.Import
	for _, p := range importedByPaths {
		importedBy = append(importedBy, &internal.Import{
			Name: packageName("main", p),
			Path: p,
		})
	}
	return &ImportedByDetails{
		ImportedBy: importedBy,
	}, nil
}

// readmeHTML sanitizes readmeContents based on bluemondy.UGCPolicy and returns
// a template.HTML. If readmeFilePath indicates that this is a markdown file,
// it will also render the markdown contents using blackfriday.
func readmeHTML(readmeFilePath string, readmeContents []byte) template.HTML {
	if filepath.Ext(readmeFilePath) != ".md" {
		return template.HTML(fmt.Sprintf(`<pre class="readme">%s</pre>`, html.EscapeString(string(readmeContents))))
	}

	// bluemonday.UGCPolicy allows a broad selection of HTML elements and
	// attributes that are safe for user generated content. This policy does
	// not whitelist iframes, object, embed, styles, script, etc.
	p := bluemonday.UGCPolicy()
	unsafe := blackfriday.Run(readmeContents)
	return template.HTML(string(p.SanitizeBytes(unsafe)))
}

// tabSettings defines rendering options associated to each tab.  Any tab value
// not present in this map will be handled as a request to 'overview'.
var tabSettings = map[string]struct {
	alwaysShowDetails bool
}{
	"doc":        {},
	"importedby": {alwaysShowDetails: true},
	"imports":    {alwaysShowDetails: true},
	"licenses":   {},
	"module":     {},
	"overview":   {},
	"versions":   {alwaysShowDetails: true},
}

// fetchDetails returns tab details by delegating to the correct detail
// handler.
func fetchDetails(ctx context.Context, tab string, db *postgres.DB, pkg *internal.VersionedPackage) (interface{}, error) {
	switch tab {
	case "doc":
		return fetchDocumentationDetails(ctx, db, pkg)
	case "versions":
		return fetchVersionsDetails(ctx, db, &pkg.Package)
	case "module":
		return fetchModuleDetails(ctx, db, pkg)
	case "imports":
		return fetchImportsDetails(ctx, db, pkg)
	case "importedby":
		return fetchImportedByDetails(ctx, db, &pkg.Package)
	case "licenses":
		return fetchLicensesDetails(ctx, db, pkg)
	case "overview":
		return fetchOverviewDetails(ctx, db, pkg)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// HandleDetails applies database data to the appropriate template. Handles all
// endpoints that match "/" or "/<import-path>[@<version>?tab=<tab>]"
func (c *Controller) HandleDetails(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		c.renderPage(w, "index.tmpl", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if err := module.CheckImportPath(path); err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Printf("Malformed path %q: %v", path, err)
		return
	}

	version := r.FormValue("v")
	if version != "" && !semver.IsValid(version) {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		log.Printf("Malformed version %q", version)
		return
	}

	var (
		pkg *internal.VersionedPackage
		err error
		ctx = r.Context()
	)

	if version == "" {
		pkg, err = c.db.GetLatestPackage(ctx, path)
	} else {
		pkg, err = c.db.GetPackage(ctx, path, version)
	}
	if err != nil {
		if derrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			c.renderPage(w, "package404.tmpl", nil)
			return
		}
		log.Printf("error getting package for %s@%s: %v", path, version, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	version = pkg.VersionInfo.Version
	pkgHeader, err := createPackageHeader(pkg)
	if err != nil {
		log.Printf("error creating package header for %s@%s: %v", path, version, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	tab := r.FormValue("tab")
	settings, ok := tabSettings[tab]
	if !ok {
		tab = "overview"
		settings = tabSettings["overview"]
	}
	canShowDetails := pkg.IsRedistributable() || settings.alwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetails(ctx, tab, c.db, pkg)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("error fetching page for %q: %v", tab, err)
			return
		}
	}

	page := &DetailsPage{
		PackageHeader:  pkgHeader,
		Details:        details,
		CanShowDetails: canShowDetails,
	}

	c.renderPage(w, tab+".tmpl", page)
}
