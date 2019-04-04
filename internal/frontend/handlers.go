// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"gopkg.in/russross/blackfriday.v2"
)

// ModulePage contains all of the data that the overview template
// needs to populate.
type ModulePage struct {
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

// fetchModulePage fetches data for the module version specified by path and version
// from the database and returns a ModulePage.
func fetchModulePage(db *postgres.DB, path, version string) (*ModulePage, error) {
	pkg, err := db.GetPackage(path, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetPackage(%q, %q) returned error %v", path, version, err)
	}

	return &ModulePage{
		ModulePath: pkg.Version.Module.Path,
		ReadMe:     readmeHTML(pkg.Version.ReadMe),
		PackageHeader: &PackageHeader{
			Name:       pkg.Name,
			Version:    pkg.Version.Version,
			Path:       pkg.Path,
			Synopsis:   pkg.Synopsis,
			License:    pkg.Version.License,
			CommitTime: elapsedTime(pkg.Version.CommitTime),
		},
	}, nil
}

func readmeHTML(readme []byte) template.HTML {
	unsafe := blackfriday.Run(readme)
	b := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return template.HTML(string(b))
}

// MakeModuleHandlerFunc uses a module page that contains module data from
// a database and applies that data and html to a template.
func MakeModuleHandlerFunc(db *postgres.DB, html string, templates *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, version, err := parseModulePathAndVersion(r.URL.Path)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			log.Printf("Error parsing path and version: %v", err)
			return
		}

		modPage, err := fetchModulePage(db, path, version)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("Error fetching module page: %v", err)
			return
		}

		var buf bytes.Buffer
		if err := templates.ExecuteTemplate(&buf, html, modPage); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("Error executing module page template: %v", err)
			return
		}
		if _, err := io.Copy(w, &buf); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("Error copying template buffer to ResponseWriter: %v", err)
			return
		}
	}
}

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
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
	CommitTime   time.Time
	NumImporters int64
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(db *postgres.DB, query string) (*SearchPage, error) {
	terms := strings.Fields(query)
	dbresults, err := db.Search(terms)
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
			CommitTime:   r.Package.Version.CommitTime,
			NumImporters: r.NumImporters,
		})
	}

	return &SearchPage{Results: results}, nil
}
