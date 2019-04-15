// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
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

// OverviewPage contains all of the data that the overview template
// needs to populate.
type OverviewPage struct {
	ModulePath    string
	ReadMe        template.HTML
	PackageHeader *Package
}

// ModulePage contains all of the data that the module template
// needs to populate.
type ModulePage struct {
	ModulePath    string
	Version       string
	ReadMe        template.HTML
	Packages      []*Package
	PackageHeader *Package
}

// Package contains information for an individual package.
type Package struct {
	Version    string
	Path       string
	ModulePath string
	Synopsis   string
	License    string
	CommitTime string
	Name       string
}

// Dir returns the directory of the package relative to the root of the module.
func (p *Package) Dir() string {
	return strings.TrimPrefix(p.Path, fmt.Sprintf("%s/", p.ModulePath))
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

// VersionsPage contains all the data that the versions tab
// template needs to populate.
type VersionsPage struct {
	Versions      []*MajorVersionGroup
	PackageHeader *Package
}

// parsePageTemplates parses html templates contained in the given base
// directory in order to generate a map of pageName->*template.Template.
//
// Separate templates are used so that certain contextual functions (e.g.
// pageName) can be bound independently for each page.
func parsePageTemplates(base string) (map[string]*template.Template, error) {
	pages := []string{
		"index",
		"module",
		"overview",
		"search",
		"versions",
	}
	templates := make(map[string]*template.Template)
	// Loop through and create a template for each page.  This template includes
	// the page html template contained in pages/<page>.tmpl, along with all
	// helper snippets contained in helpers/*.tmpl.
	for _, pageName := range pages {
		pn := pageName
		t := template.New("").Funcs(template.FuncMap{
			"pageName": func() string { return pn },
		})
		helperGlob := filepath.Join(base, "helpers", "*.tmpl")
		if _, err := t.ParseGlob(helperGlob); err != nil {
			return nil, fmt.Errorf("ParseGlob(%q): %v", helperGlob, err)
		}
		templateName := fmt.Sprintf("%s.tmpl", pageName)
		templateFile := filepath.Join(base, "pages", templateName)
		if _, err := t.ParseFiles(templateFile); err != nil {
			return nil, fmt.Errorf("ParseFiles(%q): %v", templateFile, err)
		}
		templates[templateName] = t
	}
	return templates, nil
}

// Controller handles requests for the various frontend pages.
type Controller struct {
	db        *postgres.DB
	templates map[string]*template.Template
}

// New creates a new Controller for the given database and template directory.
func New(db *postgres.DB, templateDir string) (*Controller, error) {
	ts, err := parsePageTemplates(templateDir)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	return &Controller{
		db:        db,
		templates: ts,
	}, nil
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

// createPackageHeader returns a *Package based on the fields
// of the specified package. It assumes that pkg is not nil.
func createPackageHeader(pkg *internal.Package) (*Package, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}
	if pkg.Version == nil {
		return nil, fmt.Errorf("package's version cannot be nil")
	}

	if pkg.Version.License == "" {
		pkg.Version.License = "Missing License"
	}

	return &Package{
		Name:       pkg.Name,
		Version:    pkg.Version.Version,
		Path:       pkg.Path,
		Synopsis:   pkg.Synopsis,
		License:    pkg.Version.License,
		CommitTime: elapsedTime(pkg.Version.CommitTime),
	}, nil
}

// fetchOverviewPage fetches data for the module version specified by path and version
// from the database and returns a OverviewPage.
func fetchOverviewPage(ctx context.Context, db *postgres.DB, path, version string) (*OverviewPage, error) {
	pkg, err := db.GetPackage(ctx, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetPackage(ctx, %q, %q): %v", path, version, err)
	}

	pkgHeader, err := createPackageHeader(pkg)
	if err != nil {
		return nil, fmt.Errorf("createPackageHeader(%+v): %v", pkg, err)
	}
	return &OverviewPage{
		ModulePath:    pkg.Version.Module.Path,
		ReadMe:        readmeHTML(pkg.Version.ReadMe),
		PackageHeader: pkgHeader,
	}, nil
}

// fetchModulePage fetches data for the module version specified by pkgPath and pkgversion
// from the database and returns a ModulePage.
func fetchModulePage(ctx context.Context, db *postgres.DB, pkgPath, pkgversion string) (*ModulePage, error) {
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
			Version:    version.Version,
			ModulePath: version.Module.Path,
		})

		if p.Path == pkgPath {
			p.Version = &internal.Version{
				Version:    version.Version,
				License:    version.License,
				CommitTime: version.CommitTime,
			}
			pkgHeader, err = createPackageHeader(p)
			if err != nil {
				return nil, fmt.Errorf("createPackageHeader(%+v): %v", p, err)
			}
		}
	}

	return &ModulePage{
		ModulePath:    version.Module.Path,
		Version:       pkgversion,
		ReadMe:        readmeHTML(version.ReadMe),
		Packages:      packages,
		PackageHeader: pkgHeader,
	}, nil
}

// fetchVersionsPage fetches data for the module version specified by path and version
// from the database and returns a VersionsPage.
func fetchVersionsPage(ctx context.Context, db *postgres.DB, path, version string) (*VersionsPage, error) {
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
				Version: &internal.Version{
					Version:    version,
					License:    v.License,
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

	return &VersionsPage{
		Versions:      mvg,
		PackageHeader: pkgHeader,
	}, nil
}

func readmeHTML(readme []byte) template.HTML {
	unsafe := blackfriday.Run(readme)
	b := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return template.HTML(string(b))
}

func (c *Controller) renderPage(w http.ResponseWriter, templateName string, page interface{}) {
	var buf bytes.Buffer
	if err := c.templates[templateName].ExecuteTemplate(&buf, templateName, page); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("Error executing page template %q: %v", templateName, err)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("Error copying template %q buffer to ResponseWriter: %v", templateName, err)
	}
}

// HandleSearch applies database data to the search template. Handles endpoint
// /search?q=<query>.
func (c *Controller) HandleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.FormValue("q"))
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	page, err := fetchSearchPage(ctx, c.db, query)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Printf("fetchSearchPage(ctx, db, %q): %v", query, err)
		return
	}

	c.renderPage(w, "search.tmpl", page)
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
		html string
		page interface{}
		err  error
		ctx  = r.Context()
	)
	switch tab := r.FormValue("tab"); tab {
	case "versions":
		html = "versions.tmpl"
		page, err = fetchVersionsPage(ctx, c.db, path, version)
	case "module":
		html = "module.tmpl"
		page, err = fetchModulePage(ctx, c.db, path, version)
	case "overview":
		fallthrough
	default:
		html = "overview.tmpl"
		page, err = fetchOverviewPage(ctx, c.db, path, version)
	}

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("error fetching page for %q: %v", html, err)
		return
	}

	c.renderPage(w, html, page)
}

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	Query   string
	Results []*SearchResult
}

// SearchResult contains data needed to display a single search result.
type SearchResult struct {
	Name         string
	PackagePath  string
	ModulePath   string
	Synopsis     string
	Version      string
	License      string
	CommitTime   string
	NumImporters int64
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string) (*SearchPage, error) {
	terms := strings.Fields(query)
	dbresults, err := db.Search(ctx, terms)
	if err != nil {
		return nil, fmt.Errorf("db.Search(%v): %v", terms, err)
	}

	var results []*SearchResult
	for _, r := range dbresults {
		results = append(results, &SearchResult{
			Name:         r.Package.Name,
			PackagePath:  r.Package.Path,
			ModulePath:   r.Package.Version.Module.Path,
			Synopsis:     r.Package.Synopsis,
			Version:      r.Package.Version.Version,
			License:      r.Package.Version.License,
			CommitTime:   elapsedTime(r.Package.Version.CommitTime),
			NumImporters: r.NumImporters,
		})
	}

	return &SearchPage{
		Query:   query,
		Results: results,
	}, nil
}
