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
	"sync"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

// Server can be installed to serve the go discovery frontend.
type Server struct {
	ds              DataSource
	staticPath      string
	templateDir     string
	reloadTemplates bool
	errorPage       []byte

	mu        sync.RWMutex // Protects all fields below
	templates map[string]*template.Template
}

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetDirectory returns packages whose import path is in a (possibly nested)
	// subdirectory of the given directory path. If version is the empty string,
	// directory operates on the latest version.
	GetDirectory(ctx context.Context, dirPath, version string) (_ *internal.Directory, err error)
	// GetImportedBy returns a slice of import paths corresponding to packages
	// that import the given package path (at any version).
	GetImportedBy(ctx context.Context, path, version string, limit int) ([]string, error)
	// GetImports returns a slice of import paths imported by the package
	// specified by path and version.
	GetImports(ctx context.Context, path, version string) ([]string, error)
	// GetModuleLicenses returns all top-level Licenses for the given modulePath
	// and version. (i.e., Licenses contained in the module root directory)
	GetModuleLicenses(ctx context.Context, modulePath, version string) ([]*license.License, error)
	// GetPackage returns the VersionedPackage corresponding to the given package
	// path and version. When multiple package paths satisfy this query, it
	// should prefer the module with the longest path.
	GetPackage(ctx context.Context, path, version string) (*internal.VersionedPackage, error)
	// GetPackageLicenses returns all Licenses that apply to pkgPath, within the
	// module version specified by modulePath and version.
	GetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) ([]*license.License, error)
	// GetPackagesInVersion returns Packages contained in the module version
	// specified by modulePath and version.
	GetPackagesInVersion(ctx context.Context, modulePath, version string) ([]*internal.Package, error)
	// GetPseudoVersionsForModule returns VersionInfo for all known
	// pseudo-versions for the module corresponding to modulePath.
	GetPseudoVersionsForModule(ctx context.Context, modulePath string) ([]*internal.VersionInfo, error)
	// GetPseudoVersionsForModule returns VersionInfo for all known
	// pseudo-versions for any module containing a package with the given import
	// path.
	GetPseudoVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error)
	// GetTaggedVersionsForModule returns VersionInfo for all known tagged
	// versions for the module corresponding to modulePath.
	GetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*internal.VersionInfo, error)
	// GetTaggedVersionsForModule returns VersionInfo for all known tagged
	// versions for any module containing a package with the given import path.
	GetTaggedVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error)
	// GetVersionInfo returns the VersionInfo corresponding to modulePath and
	// version.
	GetVersionInfo(ctx context.Context, modulePath, version string) (*internal.VersionInfo, error)
	// LegacySearch performs a search for the given query, with pagination
	// specified by limit and offset.
	LegacySearch(ctx context.Context, query string, limit, offset int) ([]*postgres.SearchResult, error)
}

// NewServer creates a new Server for the given database and template directory.
// reloadTemplates should be used during development when it can be helpful to
// reload templates from disk each time a page is loaded.
func NewServer(ds DataSource, staticPath string, reloadTemplates bool) (*Server, error) {
	templateDir := filepath.Join(staticPath, "html")
	ts, err := parsePageTemplates(templateDir)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	s := &Server{
		ds:              ds,
		staticPath:      staticPath,
		templateDir:     templateDir,
		reloadTemplates: reloadTemplates,
		templates:       ts,
	}
	errorPageBytes, err := s.renderErrorPage(http.StatusInternalServerError, nil)
	if err != nil {
		return nil, fmt.Errorf("s.renderErrorPage(http.StatusInternalServerError, nil): %v", err)
	}
	s.errorPage = errorPageBytes
	return s, nil
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler)) {
	handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticPath))))
	handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("%s/img/favicon.ico", http.Dir(s.staticPath)))
	}))
	handle("/pkg/", http.HandlerFunc(s.handlePackageDetails))
	handle("/mod/", http.HandlerFunc(s.handleModuleDetails))
	handle("/search", http.HandlerFunc(s.handleSearch))
	handle("/search-help", s.handleStaticPage("search_help.tmpl", "Search Help - Go Discovery"))
	handle("/license-policy", s.handleStaticPage("license_policy.tmpl", "Licenses - Go Discovery"))
	handle("/copyright", s.handleStaticPage("copyright.tmpl", "Copyright - Go Discovery"))
	handle("/tos", s.handleStaticPage("tos.tmpl", "Terms of Service - Go Discovery"))
	handle("/", http.HandlerFunc(s.handleIndexPage))
}

// handleIndexPage handles requests to the index page.
func (s *Server) handleIndexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.handleStaticPage("index.tmpl", "Go Discovery")(w, r)
		return
	}

	query := strings.TrimPrefix(r.URL.Path, "/")
	s.serveErrorPage(w, r, http.StatusNotFound, &errorPage{
		Message:          fmt.Sprintf("%d %s", http.StatusNotFound, http.StatusText(http.StatusNotFound)),
		SecondaryMessage: suggestedSearch(query),
	})
}

func suggestedSearch(userInput string) template.HTML {
	safe := template.HTMLEscapeString(userInput)
	return template.HTML(fmt.Sprintf(`To search for packages like %q, <a href="/search?q=%s">click here</a>.</p>`, safe, safe))
}

// handleStaticPage handles requests to a template that contains no dynamic
// content.
func (s *Server) handleStaticPage(templateName, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.servePage(w, templateName, newBasePage(r, title))
	}
}

// basePage contains fields shared by all pages when rendering templates.
type basePage struct {
	Title string
	Query string
	Nonce string
}

// newBasePage returns a base page for the given request and title.
func newBasePage(r *http.Request, title string) basePage {
	nonce, ok := middleware.GetNonce(r.Context())
	if !ok {
		log.Printf("middleware.GetNonce: nonce was not set")
	}
	return basePage{
		Title: title,
		Query: searchQuery(r),
		Nonce: nonce,
	}
}

// GoogleAnalyticsTrackingID returns the tracking ID from
// func (b basePage) GoogleAnalyticsTrackingID() string {
	return "UA-141356704-1"
}

// AppVersionLabel uniquely identifies the currently running binary. It can be
// used for cache-busting query parameters.
func (b basePage) AppVersionLabel() string {
	return config.AppVersionLabel()
}

// errorPage contains fields for rendering a HTTP error page.
type errorPage struct {
	basePage
	Message          string
	SecondaryMessage template.HTML
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, page *errorPage) {
	if page == nil {
		page = &errorPage{
			basePage: newBasePage(r, ""),
		}
	}
	buf, err := s.renderErrorPage(status, page)
	if err != nil {
		log.Printf("s.renderErrorPage(w, %d, %v): %v", status, page, err)
		buf = s.errorPage
		status = http.StatusInternalServerError
	}

	w.WriteHeader(status)
	if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
		log.Printf("Error copying template %q buffer to ResponseWriter: %v", "error.tmpl", err)
	}
}

// renderErrorPage executes error.tmpl with the given errorPage
func (s *Server) renderErrorPage(status int, page *errorPage) ([]byte, error) {
	statusInfo := fmt.Sprintf("%d %s", status, http.StatusText(status))
	if page == nil {
		page = &errorPage{
			Message: statusInfo,
			basePage: basePage{
				Title: statusInfo,
			},
		}
	}
	if page.Message == "" {
		page.Message = statusInfo
	}
	if page.Title == "" {
		page.Title = statusInfo
	}
	return s.renderPage("error.tmpl", page)
}

// servePage is used to execute all templates for a *Server.
func (s *Server) servePage(w http.ResponseWriter, templateName string, page interface{}) {
	if s.reloadTemplates {
		s.mu.Lock()
		var err error
		s.templates, err = parsePageTemplates(s.templateDir)
		s.mu.Unlock()
		if err != nil {
			log.Printf("Error parsing templates: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	buf, err := s.renderPage(templateName, page)
	if err != nil {
		log.Printf("s.renderPage(%q, %+v): %v", templateName, page, err)
		w.WriteHeader(http.StatusInternalServerError)
		buf = s.errorPage
	}
	if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
		log.Printf("Error copying template %q buffer to ResponseWriter: %v", templateName, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// renderPage executes the given templateName with page.
func (s *Server) renderPage(templateName string, page interface{}) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var buf bytes.Buffer
	tmpl := s.templates[templateName]
	if tmpl == nil {
		return nil, fmt.Errorf("BUG: s.templates[%q] not found", templateName)
	}
	if err := tmpl.Execute(&buf, page); err != nil {
		log.Printf("Error executing page template %q: %v", templateName, err)
		return nil, err

	}
	return buf.Bytes(), nil
}

// parsePageTemplates parses html templates contained in the given base
// directory in order to generate a map of Name->*template.Template.
//
// Separate templates are used so that certain contextual functions (e.g.
// templateName) can be bound independently for each page.
func parsePageTemplates(base string) (map[string]*template.Template, error) {
	htmlSets := [][]string{
		{"index.tmpl"},
		{"error.tmpl"},
		{"search.tmpl"},
		{"search_help.tmpl"},
		{"copyright.tmpl"},
		{"license_policy.tmpl"},
		{"tos.tmpl"},
		{"directory.tmpl"},
		{"readme.tmpl", "details.tmpl"},
		{"module.tmpl", "details.tmpl"},
		{"pkg_doc.tmpl", "details.tmpl"},
		{"pkg_importedby.tmpl", "details.tmpl"},
		{"pkg_imports.tmpl", "details.tmpl"},
		{"licenses.tmpl", "details.tmpl"},
		{"versions.tmpl", "details.tmpl"},
		{"not_implemented.tmpl", "details.tmpl"},
	}

	templates := make(map[string]*template.Template)
	for _, set := range htmlSets {
		t, err := template.New("base.tmpl").Funcs(template.FuncMap{
			"add": func(i, j int) int { return i + j },
			"pluralize": func(i int, s string) string {
				if i == 1 {
					return s
				}
				return s + "s"
			},
		}).ParseFiles(filepath.Join(base, "base.tmpl"))
		if err != nil {
			return nil, fmt.Errorf("ParseFiles: %v", err)
		}
		helperGlob := filepath.Join(base, "helpers", "*.tmpl")
		if _, err := t.ParseGlob(helperGlob); err != nil {
			return nil, fmt.Errorf("ParseGlob(%q): %v", helperGlob, err)
		}

		var files []string
		for _, f := range set {
			files = append(files, filepath.Join(base, "pages", f))
		}
		if _, err := t.ParseFiles(files...); err != nil {
			return nil, fmt.Errorf("ParseFiles(%v): %v", files, err)
		}
		templates[set[0]] = t
	}
	return templates, nil
}
