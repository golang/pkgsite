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
	"net/url"
	"strings"
	"time"

	"golang.org/x/discovery/internal/postgres"
)

// ModulePage contains all of the data that the overview template
// needs to populate.
type ModulePage struct {
	Path       string
	Version    string
	License    string
	CommitTime string
	ReadMe     string
}

// parseModulePathAndVersion returns the module and version specified by u. u is
// assumed to be a valid url following the structure
// https://<frontendHost>/<module>?v=<version>&tab=<tab>.
func parseModulePathAndVersion(u *url.URL) (name, version string, err error) {
	name = strings.TrimPrefix(u.Path, "/")
	versionQuery := u.Query()["v"]
	if name == "" || len(versionQuery) != 1 || versionQuery[0] == "" {
		return "", "", fmt.Errorf("invalid path: %q", u)
	}
	return name, versionQuery[0], nil
}

// elapsedTime takes a date and returns returns human-readable,
// relative timestamps based on the following rules:
// (1) 'X hours ago' when X < 6
// (2) 'today' between 6 hours and 1 day ago
// (3) 'Y days ago' when Y < 6
// (4) A date formatted like "Jan 2, 2006" for anything further back
func elapsedTime(date time.Time) string {
	elapsedHours := int(time.Now().Sub(date).Hours())
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

// fetchModulePage fetches data for the module version specified by name and version
// from the database and returns a ModulePage.
func fetchModulePage(db *postgres.DB, name, version string) (*ModulePage, error) {
	ver, err := db.GetVersion(name, version)
	if err != nil {
		return nil, fmt.Errorf("db.GetVersion(%q, %q) returned error %v", name, version, err)
	}

	return &ModulePage{
		Path:       ver.Module.Path,
		Version:    ver.Version,
		License:    ver.License,
		CommitTime: elapsedTime(ver.CommitTime),
		ReadMe:     ver.ReadMe,
	}, nil
}

// MakeModuleHandlerFunc uses a module page that contains module data from
// a database and applies that data and html to a template.
func MakeModuleHandlerFunc(db *postgres.DB, html string, templates *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, version, err := parseModulePathAndVersion(r.URL)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("Error parsing name and version: %v", err)
			return
		}

		modPage, err := fetchModulePage(db, name, version)
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
