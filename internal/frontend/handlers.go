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
	"reflect"
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
	PackageHeader *PackageHeader
}

// PackageHeader contains all of the data the header template
// needs to populate.
type PackageHeader struct {
	Name       string
	Version    string
	Path       string
	Synopsis   string
	License    string
	CommitTime string
}

// MajorVersionGroup represents the major level of the versions
// list hierarchy (i.e. "v1").
type MajorVersionGroup struct {
	Level    string
	Latest   *VersionInfo
	Versions []*MinorVersionGroup
}

// MinorVersionGroup represents the major/minor level of the versions
// list hierarchy (i.e. "1.5").
type MinorVersionGroup struct {
	Level    string
	Latest   *VersionInfo
	Versions []*VersionInfo
}

// VersionInfo contains the information that will be displayed
// the lowest level of the versions tab's list hierarchy.
type VersionInfo struct {
	Version     string
	PackagePath string
	CommitTime  string
}

// VersionsPage contains all the data that the versions tab
// template needs to populate.
type VersionsPage struct {
	Versions      []*MajorVersionGroup
	PackageHeader *PackageHeader
}

// parseTabTemplates parses html templates contained in the given base
// directory in order to generate a map of tabName->*template.Template.
//
// Separate templates are used so that certain contextual functions (e.g.
// tabName) can be bound independently for each tab.
func parseTabTemplates(base string) (map[string]*template.Template, error) {
	commonFuncs := template.FuncMap{
		"equal": reflect.DeepEqual,
	}
	tabs := []string{
		"index", "overview", "search", "versions",
	}
	templates := make(map[string]*template.Template)
	// Loop through and create a template for each tab.  This template includes
	// the page html template contained in pages/<tab>.tmpl, along with all
	// helper snippets contained in helpers/*.tmpl.
	for _, tabName := range tabs {
		pn := tabName
		t := template.New("").Funcs(commonFuncs).Funcs(template.FuncMap{
			"tabName": func() string { return pn },
		})
		helperGlob := filepath.Join(base, "helpers", "*.tmpl")
		if _, err := t.ParseGlob(helperGlob); err != nil {
			return nil, fmt.Errorf("ParseGlob(%q): %v", helperGlob, err)
		}
		templateName := fmt.Sprintf("%s.tmpl", tabName)
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
	ts, err := parseTabTemplates(templateDir)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	return &Controller{
		db:        db,
		templates: ts,
	}, nil
}

// parseModulePathAndVersion returns the module and version specified by p. p is
// assumed to be a valid path following the structure /<module>@<version>.
func parseModulePathAndVersion(p string) (path, version string, err error) {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path: %q", p)
	}

	if err := module.CheckPath(parts[0]); err != nil {
		return "", "", fmt.Errorf("invalid path (%q): module.CheckPath(%q): %v", p, parts[0], err)
	}

	if !semver.IsValid(parts[1]) {
		return "", "", fmt.Errorf("invalid path (%q): semver.IsValid(%q) = false", p, parts[1])
	}

	return parts[0], parts[1], nil
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

// createPackageHeader returns a *PackageHeader based on the fields
// of the specified package. It assumes that pkg is not nil.
func createPackageHeader(pkg *internal.Package) (*PackageHeader, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}
	if pkg.Version == nil {
		return nil, fmt.Errorf("package's version cannot be nil")
	}

	if pkg.Version.License == "" {
		pkg.Version.License = "Missing License"
	}

	return &PackageHeader{
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
		return nil, fmt.Errorf("db.GetPackage(ctx, %q, %q) returned error %v", path, version, err)
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

// fetchVersionsPage fetches data for the module version specified by path and version
// from the database and returns a VersionsPage.
func fetchVersionsPage(ctx context.Context, db *postgres.DB, path, version string) (*VersionsPage, error) {
	versions, err := db.GetTaggedVersionsForPackageSeries(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("db.GetTaggedVersions(%q): %v", path, err)
	}

	// if GetTaggedVersionsForPackageSeries returns nothing then that means there are no
	// tagged versions and we want to get the pseudo-versions instead
	if len(versions) == 0 {
		versions, err = db.GetPseudoVersionsForPackageSeries(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("db.GetPseudoVersions(%q): %v", path, err)
		}
	}

	var (
		pkgHeader       = &PackageHeader{}
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
				Latest: &VersionInfo{
					Version:     fullVersion,
					PackagePath: v.Packages[0].Path,
					CommitTime:  elapsedTime(v.CommitTime),
				},
				Versions: []*MinorVersionGroup{},
			})
		}

		if prevMajMin != majMin {
			prevMajMinIndex = len(mvg[prevMajorIndex].Versions)
			prevMajMin = majMin
			mvg[prevMajorIndex].Versions = append(mvg[prevMajorIndex].Versions, &MinorVersionGroup{
				Level: majMin,
				Latest: &VersionInfo{
					Version:     fullVersion,
					PackagePath: v.Packages[0].Path,
					CommitTime:  elapsedTime(v.CommitTime),
				},
			})
		}

		mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions = append(mvg[prevMajorIndex].Versions[prevMajMinIndex].Versions, &VersionInfo{
			Version:     fullVersion,
			PackagePath: v.Packages[0].Path,
			CommitTime:  elapsedTime(v.CommitTime),
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
	query := r.URL.Query().Get("q")
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
	var (
		html string
		page interface{}
		ctx  = r.Context()
	)

	if r.URL.Path == "/" {
		html = "index.tmpl"
	} else {
		path, version, err := parseModulePathAndVersion(r.URL.Path)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			log.Printf("error parsing path and version: %v", err)
			return
		}

		switch tab := r.URL.Query().Get("tab"); tab {
		case "versions":
			html = "versions.tmpl"
			page, err = fetchVersionsPage(ctx, c.db, path, version)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				log.Printf("error fetching module page: %v", err)
				return
			}
		case "overview":
			fallthrough
		default:
			html = "overview.tmpl"
			page, err = fetchOverviewPage(ctx, c.db, path, version)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				log.Printf("error fetching module page: %v", err)
				return
			}
		}
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
		return nil, fmt.Errorf("db.Search(%v) returned error %v", terms, err)
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
