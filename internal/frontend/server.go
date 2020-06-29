// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v7"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/queue"
)

// Server can be installed to serve the go discovery frontend.
type Server struct {
	ds    internal.DataSource
	queue queue.Queue
	// cmplClient is a redis client that has access to the "completions" sorted
	// set.
	cmplClient           *redis.Client
	taskIDChangeInterval time.Duration
	staticPath           string
	thirdPartyPath       string
	templateDir          string
	devMode              bool
	errorPage            []byte
	appVersionLabel      string

	mu        sync.Mutex // Protects all fields below
	templates map[string]*template.Template
}

// ServerConfig contains everything needed by a Server.
type ServerConfig struct {
	DataSource           internal.DataSource
	Queue                queue.Queue
	CompletionClient     *redis.Client
	TaskIDChangeInterval time.Duration
	StaticPath           string
	ThirdPartyPath       string
	DevMode              bool
	AppVersionLabel      string
}

// NewServer creates a new Server for the given database and template directory.
func NewServer(scfg ServerConfig) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(...)")
	templateDir := filepath.Join(scfg.StaticPath, "html")
	ts, err := parsePageTemplates(templateDir)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	s := &Server{
		ds:                   scfg.DataSource,
		queue:                scfg.Queue,
		cmplClient:           scfg.CompletionClient,
		staticPath:           scfg.StaticPath,
		thirdPartyPath:       scfg.ThirdPartyPath,
		templateDir:          templateDir,
		devMode:              scfg.DevMode,
		templates:            ts,
		taskIDChangeInterval: scfg.TaskIDChangeInterval,
		appVersionLabel:      scfg.AppVersionLabel,
	}
	errorPageBytes, err := s.renderErrorPage(context.Background(), http.StatusInternalServerError, "error.tmpl", nil)
	if err != nil {
		return nil, fmt.Errorf("s.renderErrorPage(http.StatusInternalServerError, nil): %v", err)
	}
	s.errorPage = errorPageBytes
	return s, nil
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler), redisClient *redis.Client) {
	var (
		detailHandler http.Handler = s.errorHandler(s.serveDetails)
		searchHandler http.Handler = s.errorHandler(s.serveSearch)
	)
	if redisClient != nil {
		detailHandler = middleware.Cache("details", redisClient, detailsTTL)(detailHandler)
		searchHandler = middleware.Cache("search", redisClient, middleware.TTL(defaultTTL))(searchHandler)
	}
	handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticPath))))
	handle("/third_party/", http.StripPrefix("/third_party", http.FileServer(http.Dir(s.thirdPartyPath))))
	handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("%s/img/favicon.ico", http.Dir(s.staticPath)))
	}))
	handle("/fetch/", http.HandlerFunc(s.fetchHandler))
	handle("/pkg/", http.HandlerFunc(s.handlePackageDetailsRedirect))
	handle("/search", searchHandler)
	handle("/search-help", s.staticPageHandler("search_help.tmpl", "Search Help - go.dev"))
	handle("/license-policy", s.licensePolicyHandler())
	handle("/about", http.RedirectHandler("https://go.dev/about", http.StatusFound))
	handle("/", detailHandler)
	handle("/autocomplete", http.HandlerFunc(s.handleAutoCompletion))
	handle("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(`User-agent: *
Disallow: /search?*
Disallow: /fetch/*
`))
	}))
}

const (
	// defaultTTL is used when details tab contents are subject to change, or when
	// there is a problem confirming that the details can be permanently cached.
	defaultTTL = 1 * time.Hour
	// shortTTL is used for volatile content, such as the latest version of a
	// package or module.
	shortTTL = 10 * time.Minute
	// longTTL is used when details content is essentially static.
	longTTL = 24 * time.Hour
)

// detailsTTL assigns the cache TTL for package detail requests.
func detailsTTL(r *http.Request) time.Duration {
	return detailsTTLForPath(r.Context(), r.URL.Path, r.FormValue("tab"))
}

func detailsTTLForPath(ctx context.Context, urlPath, tab string) time.Duration {
	if urlPath == "/" {
		return defaultTTL
	}
	if strings.HasPrefix(urlPath, "/mod") {
		urlPath = strings.TrimPrefix(urlPath, "/mod")
	}
	_, _, version, err := parseDetailsURLPath(urlPath)
	if err != nil {
		log.Errorf(ctx, "falling back to default TTL: %v", err)
		return defaultTTL
	}
	if version == internal.LatestVersion {
		return shortTTL
	}
	if tab == "importedby" || tab == "versions" {
		return defaultTTL
	}
	return longTTL
}

// TagRoute categorizes incoming requests to the frontend for use in
// monitoring.
func TagRoute(route string, r *http.Request) string {
	tag := strings.Trim(route, "/")
	if tab := r.FormValue("tab"); tab != "" {
		// Verify that the tab value actually exists, otherwise this is unsanitized
		// input and could result in unbounded cardinality in our metrics.
		_, pkgOK := packageTabLookup[tab]
		_, modOK := moduleTabLookup[tab]
		if pkgOK || modOK {
			if tag != "" {
				tag += "-"
			}
			tag += tab
		}
	}
	return tag
}

// staticPageHandler handles requests to a template that contains no dynamic
// content.
func (s *Server) staticPageHandler(templateName, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.servePage(r.Context(), w, templateName, s.newBasePage(r, title))
	}
}

// basePage contains fields shared by all pages when rendering templates.
type basePage struct {
	HTMLTitle       string
	Query           string
	Nonce           string
	Experiments     *experiment.Set
	GodocURL        string
	DevMode         bool
	AppVersionLabel string
}

// licensePolicyPage is used to generate the static license policy page.
type licensePolicyPage struct {
	basePage
	LicenseFileNames []string
	LicenseTypes     []licenses.AcceptedLicenseInfo
}

func (s *Server) licensePolicyHandler() http.HandlerFunc {
	lics := licenses.AcceptedLicenses()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := licensePolicyPage{
			basePage:         s.newBasePage(r, "Licenses"),
			LicenseFileNames: licenses.FileNames,
			LicenseTypes:     lics,
		}
		s.servePage(r.Context(), w, "license_policy.tmpl", page)
	})
}

// newBasePage returns a base page for the given request and title.
func (s *Server) newBasePage(r *http.Request, title string) basePage {
	return basePage{
		HTMLTitle:       title,
		Query:           searchQuery(r),
		Nonce:           middleware.NoncePlaceholder,
		Experiments:     experiment.FromContext(r.Context()),
		GodocURL:        middleware.GodocURLPlaceholder,
		DevMode:         s.devMode,
		AppVersionLabel: s.appVersionLabel,
	}
}

// GoogleTagManagerContainerID returns the container ID from GoogleTagManager.
func (b basePage) GoogleTagManagerContainerID() string {
	return "GTM-W8MVQXG"
}

// errorPage contains fields for rendering a HTTP error page.
type errorPage struct {
	basePage
	templateName    string
	messageTemplate string
	MessageData     interface{}
}

// PanicHandler returns an http.HandlerFunc that can be used in HTTP
// middleware. It returns an error if something goes wrong pre-rendering the
// error template.
func (s *Server) PanicHandler() (_ http.HandlerFunc, err error) {
	defer derrors.Wrap(&err, "PanicHandler")
	status := http.StatusInternalServerError
	buf, err := s.renderErrorPage(context.Background(), status, "error.tmpl", nil)
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
			log.Errorf(r.Context(), "Error copying panic template to ResponseWriter: %v", err)
		}
	}, nil
}

type serverError struct {
	status int // HTTP status code
	epage  *errorPage
	err    error // wrapped error
}

func (s *serverError) Error() string {
	return fmt.Sprintf("%d (%s): %v (epage=%v)", s.status, http.StatusText(s.status), s.err, s.epage)
}

func (s *serverError) Unwrap() error {
	return s.err
}

func (s *Server) errorHandler(f func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			s.serveError(w, r, err)
		}
	}
}

func (s *Server) serveError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	var serr *serverError
	if !errors.As(err, &serr) {
		serr = &serverError{status: http.StatusInternalServerError, err: err}
	}
	if serr.status == http.StatusInternalServerError {
		log.Error(ctx, err)
	} else {
		log.Infof(ctx, "returning %d (%s) for error %v", serr.status, http.StatusText(serr.status), err)
	}
	s.serveErrorPage(w, r, serr.status, serr.epage)
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, page *errorPage) {
	template := "error.tmpl"
	if page == nil {
		page = &errorPage{
			basePage: s.newBasePage(r, ""),
		}
	} else if page.templateName != "" {
		template = page.templateName
	}
	buf, err := s.renderErrorPage(r.Context(), status, template, page)
	if err != nil {
		log.Errorf(r.Context(), "s.renderErrorPage(w, %d, %v): %v", status, page, err)
		buf = s.errorPage
		status = http.StatusInternalServerError
	}

	w.WriteHeader(status)
	if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
		log.Errorf(r.Context(), "Error copying template %q buffer to ResponseWriter: %v", template, err)
	}
}

// renderErrorPage executes error.tmpl with the given errorPage
func (s *Server) renderErrorPage(ctx context.Context, status int, templateName string, page *errorPage) ([]byte, error) {
	statusInfo := fmt.Sprintf("%d %s", status, http.StatusText(status))
	if page == nil {
		page = &errorPage{}
	}
	if page.Nonce == "" {
		page.Nonce = middleware.NoncePlaceholder
	}
	if page.messageTemplate == "" {
		page.messageTemplate = `<h3 class="Error-message">{{.}}</h3>`
	}
	if page.MessageData == nil {
		page.MessageData = statusInfo
	}
	if page.HTMLTitle == "" {
		page.HTMLTitle = statusInfo
	}
	if templateName == "" {
		templateName = "error.tmpl"
	}

	etmpl, err := s.findTemplate(templateName)
	if err != nil {
		return nil, err
	}
	tmpl, err := etmpl.Clone()
	if err != nil {
		return nil, err
	}
	_, err = tmpl.New("message").Parse(page.messageTemplate)
	if err != nil {
		return nil, err
	}

	return executeTemplate(ctx, templateName, tmpl, page)
}

// servePage is used to execute all templates for a *Server.
func (s *Server) servePage(ctx context.Context, w http.ResponseWriter, templateName string, page interface{}) {
	buf, err := s.renderPage(ctx, templateName, page)
	if err != nil {
		log.Errorf(ctx, "s.renderPage(%q, %+v): %v", templateName, page, err)
		w.WriteHeader(http.StatusInternalServerError)
		buf = s.errorPage
	}
	if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
		log.Errorf(ctx, "Error copying template %q buffer to ResponseWriter: %v", templateName, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// renderPage executes the given templateName with page.
func (s *Server) renderPage(ctx context.Context, templateName string, page interface{}) ([]byte, error) {
	tmpl, err := s.findTemplate(templateName)
	if err != nil {
		return nil, err
	}
	return executeTemplate(ctx, templateName, tmpl, page)
}

func (s *Server) findTemplate(templateName string) (*template.Template, error) {
	if s.devMode {
		s.mu.Lock()
		defer s.mu.Unlock()
		var err error
		s.templates, err = parsePageTemplates(s.templateDir)
		if err != nil {
			return nil, fmt.Errorf("error parsing templates: %v", err)
		}
	}
	tmpl := s.templates[templateName]
	if tmpl == nil {
		return nil, fmt.Errorf("BUG: s.templates[%q] not found", templateName)
	}
	return tmpl, nil
}

func executeTemplate(ctx context.Context, templateName string, tmpl *template.Template, data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Errorf(ctx, "Error executing page template %q: %v", templateName, err)
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
		{"notfound.tmpl"},
		{"search.tmpl"},
		{"search_help.tmpl"},
		{"license_policy.tmpl"},
		{"overview.tmpl", "details.tmpl"},
		{"subdirectories.tmpl", "details.tmpl"},
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
			"commaseparate": func(s []string) string {
				return strings.Join(s, ", ")
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
