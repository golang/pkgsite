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
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// DetailsPage contains data for the doc template.
type DetailsPage struct {
	Details       interface{}
	PackageHeader *Package
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

// ImportedByDetails contains information for all packages that import a given
// package.
type ImportedByDetails struct {
	// ImportedBy is an array of the package paths that import a given
	// package.
	ImportedBy []string
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
	Name       string
	Version    string
	Path       string
	ModulePath string
	Synopsis   string
	CommitTime string
	Title      string
	Licenses   []LicenseInfo
	IsCommand  bool
}

// Dir returns the directory of the package relative to the root of the module.
func (p *Package) Dir() string {
	return strings.TrimPrefix(p.Path, fmt.Sprintf("%s/", p.ModulePath))
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
func createPackageHeader(pkg *internal.Package) (*Package, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}
	if pkg.Version == nil {
		return nil, fmt.Errorf("package's version cannot be nil")
	}

	var isCmd bool
	if pkg.Name == "main" {
		isCmd = true
	}
	name := packageName(pkg.Name, pkg.Path)
	return &Package{
		Name:       name,
		IsCommand:  isCmd,
		Title:      packageTitle(name, isCmd),
		Version:    pkg.Version.Version,
		Path:       pkg.Path,
		Synopsis:   pkg.Synopsis,
		Licenses:   transformLicenseInfos(pkg.Licenses),
		CommitTime: elapsedTime(pkg.Version.CommitTime),
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

func fetchPackageHeader(ctx context.Context, db *postgres.DB, path, version string) (*Package, error) {
	pkg, err := db.GetPackage(ctx, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetPackage(ctx, %q, %q): %v", path, version, err)
	}

	pkgHeader, err := createPackageHeader(pkg)
	if err != nil {
		return nil, fmt.Errorf("createPackageHeader(%+v): %v", pkg, err)
	}
	return pkgHeader, nil
}

// fetchOverviewDetails fetches data for the module version specified by path and version
// from the database and returns a OverviewDetails.
func fetchOverviewDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	pkg, err := db.GetPackage(ctx, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetPackage(ctx, %q, %q): %v", path, version, err)
	}

	pkgHeader, err := createPackageHeader(pkg)
	if err != nil {
		return nil, fmt.Errorf("createPackageHeader(%+v): %v", pkg, err)
	}
	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &OverviewDetails{
			ModulePath: pkg.Version.Module.Path,
			ReadMe:     readmeHTML(pkg.Version.ReadMe),
		},
	}, nil
}

// fetchDocumentationDetails fetches data for the package specified by path and version
// from the database and returns a DocumentationDetails.
func fetchDocumentationDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	pkgHeader, err := fetchPackageHeader(ctx, db, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.fetchPackageHeader(ctx, db, %q, %q): %v", path, version, err)
	}
	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &DocumentationDetails{
			ModulePath: pkgHeader.ModulePath,
		},
	}, nil
}

// fetchModuleDetails fetches data for the module version specified by pkgPath and pkgversion
// from the database and returns a ModuleDetails.
func fetchModuleDetails(ctx context.Context, db *postgres.DB, pkgPath, pkgversion string) (*DetailsPage, error) {
	version, err := db.GetVersionForPackage(ctx, pkgPath, pkgversion)
	if err != nil {
		return nil, fmt.Errorf("db.GetVersionForPackage(ctx, %q, %q): %v", pkgPath, pkgversion, err)
	}

	var (
		pkgHeader *Package
		packages  []*Package
	)
	for _, p := range version.Packages {
		packages = append(packages, &Package{
			Name:       p.Name,
			Path:       p.Path,
			Synopsis:   p.Synopsis,
			Licenses:   transformLicenseInfos(p.Licenses),
			Version:    version.Version,
			ModulePath: version.Module.Path,
		})

		if p.Path == pkgPath {
			p.Version = &internal.Version{
				Version:    version.Version,
				CommitTime: version.CommitTime,
			}
			pkgHeader, err = createPackageHeader(p)
			if err != nil {
				return nil, fmt.Errorf("createPackageHeader(%+v): %v", p, err)
			}
		}
	}

	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &ModuleDetails{
			ModulePath: version.Module.Path,
			Version:    pkgversion,
			ReadMe:     readmeHTML(version.ReadMe),
			Packages:   packages,
		},
	}, nil
}

// fetchVersionsDetails fetches data for the module version specified by path and version
// from the database and returns a VersionsDetails.
func fetchVersionsDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	versions, err := db.GetTaggedVersionsForPackageSeries(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("db.GetTaggedVersions(%q): %v", path, err)
	}

	// If no tagged versions for the package series are found,
	// fetch the pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = db.GetPseudoVersionsForPackageSeries(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("db.GetPseudoVersions(%q): %v", path, err)
		}
	}

	var (
		pkgHeader       = &Package{}
		mvg             = []*MajorVersionGroup{}
		prevMajor       = ""
		prevMajMin      = ""
		prevMajorIndex  = -1
		prevMajMinIndex = -1
	)

	for _, v := range versions {
		vStr := v.Version
		if vStr == version {
			pkg := &internal.Package{
				Path:     path,
				Name:     v.Packages[0].Name,
				Synopsis: v.Synopsis,
				Licenses: v.Packages[0].Licenses,
				Version: &internal.Version{
					Version:    version,
					CommitTime: v.CommitTime,
				},
			}
			pkgHeader, err = createPackageHeader(pkg)
			if err != nil {
				return nil, fmt.Errorf("createPackageHeader(%+v): %v", pkg, err)
			}
		}

		major := semver.Major(vStr)
		majMin := strings.TrimPrefix(semver.MajorMinor(vStr), "v")
		fullVersion := strings.TrimPrefix(vStr, "v")

		if prevMajor != major {
			prevMajorIndex = len(mvg)
			prevMajor = major
			mvg = append(mvg, &MajorVersionGroup{
				Level: major,
				Latest: &Package{
					Version:    fullVersion,
					Path:       v.Packages[0].Path,
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
					Path:       v.Packages[0].Path,
					CommitTime: elapsedTime(v.CommitTime),
				},
			})
		}

		mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions = append(mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions, &Package{
			Version:    fullVersion,
			Path:       v.Packages[0].Path,
			CommitTime: elapsedTime(v.CommitTime),
		})
	}

	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &VersionsDetails{
			Versions: mvg,
		},
	}, nil
}

// licenseAnchor returns the anchor that should be used to jump to the specific
// license on the licenses page.
func licenseAnchor(filePath string) string {
	return url.QueryEscape(filePath)
}

// fetchLicensesDetails fetches license data for the package version specified by
// path and version from the database and returns a LicensesDetails.
func fetchLicensesDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	pkgHeader, err := fetchPackageHeader(ctx, db, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.fetchPackageHeader(ctx, db, %q, %q): %v", path, version, err)
	}
	dbLicenses, err := db.GetLicenses(ctx, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetLicenses(ctx, %q, %q): %v", path, version, err)
	}

	licenses := make([]License, len(dbLicenses))
	for i, l := range dbLicenses {
		licenses[i] = License{
			Anchor:  licenseAnchor(l.FilePath),
			License: l,
		}
	}

	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &LicensesDetails{
			Licenses: licenses,
		},
	}, nil
}

// fetchImportsDetails fetches imports for the package version specified by
// path and version from the database and returns a ImportsDetails.
func fetchImportsDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	pkgHeader, err := fetchPackageHeader(ctx, db, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.fetchPackageHeader(ctx, db, %q, %q): %v", path, version, err)
	}

	dbImports, err := db.GetImports(ctx, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetImports(ctx, %q, %q): %v", path, version, err)
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

	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &ImportsDetails{
			Imports: imports,
			StdLib:  std,
		},
	}, nil
}

// fetchImportedByDetails fetches all packages that import the package
// specified by path and returns an ImportedByDetails.
func fetchImportedByDetails(ctx context.Context, db *postgres.DB, path, version string) (*DetailsPage, error) {
	pkgHeader, err := fetchPackageHeader(ctx, db, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.fetchPackageHeader(ctx, db, %q, %q): %v", path, version, err)
	}

	importedBy, err := db.GetImportedBy(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("db.GetImportedBy(ctx, %q): %v", path, err)
	}

	return &DetailsPage{
		PackageHeader: pkgHeader,
		Details: &ImportedByDetails{
			ImportedBy: importedBy,
		},
	}, nil
}

func readmeHTML(readme []byte) template.HTML {
	unsafe := blackfriday.Run(readme)
	b := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return template.HTML(string(b))
}

// HandleDetails applies database data to the appropriate template. Handles all
// endpoints that match "/" or "/<import-path>[@<version>?tab=<tab>]"
func (c *Controller) HandleDetails(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		c.renderPage(w, "index.tmpl", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if err := module.CheckPath(path); err != nil {
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
		page *DetailsPage
		err  error
		ctx  = r.Context()
	)

	tab := r.FormValue("tab")
	switch tab {
	case "doc":
		page, err = fetchDocumentationDetails(ctx, c.db, path, version)
	case "versions":
		page, err = fetchVersionsDetails(ctx, c.db, path, version)
	case "module":
		page, err = fetchModuleDetails(ctx, c.db, path, version)
	case "imports":
		page, err = fetchImportsDetails(ctx, c.db, path, version)
	case "importedby":
		page, err = fetchImportedByDetails(ctx, c.db, path, version)
	case "licenses":
		page, err = fetchLicensesDetails(ctx, c.db, path, version)
	case "overview":
		fallthrough
	default:
		tab = "overview"
		page, err = fetchOverviewDetails(ctx, c.db, path, version)
	}

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("error fetching page for %q: %v", tab, err)
		return
	}

	c.renderPage(w, tab+".tmpl", page)
}
