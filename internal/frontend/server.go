// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package frontend provides functionality for running the pkg.go.dev site.
package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/errorreporting"
	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/static"
	"golang.org/x/pkgsite/internal/version"
)

// Server can be installed to serve the go discovery frontend.
type Server struct {
	// getDataSource should never be called from a handler. It is called only in Server.errorHandler.
	getDataSource func(context.Context) internal.DataSource
	queue         queue.Queue
	// cmplClient is a redis client that has access to the "completions" sorted
	// set.
	cmplClient           *redis.Client
	taskIDChangeInterval time.Duration
	staticPath           template.TrustedSource
	thirdPartyPath       string
	templateDir          template.TrustedSource
	devMode              bool
	errorPage            []byte
	appVersionLabel      string
	googleTagManagerID   string
	serveStats           bool
	reportingClient      *errorreporting.Client

	mu        sync.Mutex // Protects all fields below
	templates map[string]*template.Template
}

// ServerConfig contains everything needed by a Server.
type ServerConfig struct {
	// DataSourceGetter should return a DataSource on each call.
	// It should be goroutine-safe.
	DataSourceGetter     func(context.Context) internal.DataSource
	Queue                queue.Queue
	CompletionClient     *redis.Client
	TaskIDChangeInterval time.Duration
	StaticPath           template.TrustedSource
	ThirdPartyPath       string
	DevMode              bool
	AppVersionLabel      string
	GoogleTagManagerID   string
	ServeStats           bool
	ReportingClient      *errorreporting.Client
}

// NewServer creates a new Server for the given database and template directory.
func NewServer(scfg ServerConfig) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(...)")
	templateDir := template.TrustedSourceJoin(scfg.StaticPath)
	ts, err := parsePageTemplates(templateDir)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	docTemplateDir := template.TrustedSourceJoin(templateDir, template.TrustedSourceFromConstant("dochtml"))
	dochtml.LoadTemplates(docTemplateDir)
	s := &Server{
		getDataSource:        scfg.DataSourceGetter,
		queue:                scfg.Queue,
		cmplClient:           scfg.CompletionClient,
		staticPath:           scfg.StaticPath,
		thirdPartyPath:       scfg.ThirdPartyPath,
		templateDir:          templateDir,
		devMode:              scfg.DevMode,
		templates:            ts,
		taskIDChangeInterval: scfg.TaskIDChangeInterval,
		appVersionLabel:      scfg.AppVersionLabel,
		googleTagManagerID:   scfg.GoogleTagManagerID,
		serveStats:           scfg.ServeStats,
		reportingClient:      scfg.ReportingClient,
	}
	errorPageBytes, err := s.renderErrorPage(context.Background(), http.StatusInternalServerError, "error", nil)
	if err != nil {
		return nil, fmt.Errorf("s.renderErrorPage(http.StatusInternalServerError, nil): %v", err)
	}
	s.errorPage = errorPageBytes
	return s, nil
}

// Install registers server routes using the given handler registration func.
// authValues is the set of values that can be set on authHeader to bypass the
// cache.
func (s *Server) Install(handle func(string, http.Handler), redisClient *redis.Client, authValues []string) {
	var (
		detailHandler http.Handler = s.errorHandler(s.serveDetails)
		fetchHandler  http.Handler = s.errorHandler(s.serveFetch)
		searchHandler http.Handler = s.errorHandler(s.serveSearch)
	)
	if redisClient != nil {
		detailHandler = middleware.Cache("details", redisClient, detailsTTL, authValues)(detailHandler)
		searchHandler = middleware.Cache("search", redisClient, middleware.TTL(defaultTTL), authValues)(searchHandler)
	}
	// Each AppEngine instance is created in response to a start request, which
	// is an empty HTTP GET request to /_ah/start when scaling is set to manual
	// or basic, and /_ah/warmup when scaling is automatic and min_instances is
	// set. AppEngine sends this request to bring an instance into existence.
	// See details for /_ah/start at
	// https://cloud.google.com/appengine/docs/standard/go/how-instances-are-managed#startup
	// and for /_ah/warmup at
	// https://cloud.google.com/appengine/docs/standard/go/configuring-warmup-requests.
	handle("/_ah/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof(r.Context(), "Request made to %q", r.URL.Path)
	}))
	handle("/static/", s.staticHandler())
	handle("/third_party/", http.StripPrefix("/third_party", http.FileServer(http.Dir(s.thirdPartyPath))))
	handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("%s/img/favicon.ico", http.Dir(s.staticPath.String())))
	}))
	handle("/mod/", http.HandlerFunc(s.handleModuleDetailsRedirect))
	handle("/pkg/", http.HandlerFunc(s.handlePackageDetailsRedirect))
	handle("/fetch/", fetchHandler)
	handle("/play/compile", http.HandlerFunc(s.proxyPlayground))
	handle("/play/fmt", http.HandlerFunc(s.handleFmt))
	handle("/play/share", http.HandlerFunc(s.proxyPlayground))
	handle("/search", searchHandler)
	handle("/search-help", s.staticPageHandler("search-help", "Search Help"))
	handle("/license-policy", s.licensePolicyHandler())
	handle("/about", http.RedirectHandler("https://go.dev/about", http.StatusFound))
	handle("/badge/", http.HandlerFunc(s.badgeHandler))
	handle("/styleguide", http.HandlerFunc(s.errorHandler(s.serveStyleGuide)))
	handle("/C", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Package "C" is a special case: redirect to /cmd/cgo.
		// (This is what golang.org/C does.)
		http.Redirect(w, r, "/cmd/cgo", http.StatusMovedPermanently)
	}))
	handle("/", detailHandler)
	if s.serveStats {
		handle("/detail-stats/",
			middleware.Stats()(http.StripPrefix("/detail-stats", s.errorHandler(s.serveDetails))))
	}
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
	defaultTTL = 10 * time.Minute
	// shortTTL is used for volatile content, such as the latest version of a
	// package or module.
	shortTTL = 10 * time.Minute
	// longTTL is used when details content is essentially static.
	longTTL = 10 * time.Minute
	// tinyTTL is used to cache crawled pages.
	tinyTTL = 1 * time.Minute
)

var crawlers = []string{
	"+http://www.google.com/bot.html",
	"+http://www.bing.com/bingbot.htm",
	"+http://ahrefs.com/robot",
}

// detailsTTL assigns the cache TTL for package detail requests.
func detailsTTL(r *http.Request) time.Duration {
	userAgent := r.Header.Get("User-Agent")
	for _, c := range crawlers {
		if strings.Contains(userAgent, c) {
			return tinyTTL
		}
	}
	return detailsTTLForPath(r.Context(), r.URL.Path, r.FormValue("tab"))
}

func detailsTTLForPath(ctx context.Context, urlPath, tab string) time.Duration {
	if urlPath == "/" {
		return defaultTTL
	}
	info, err := parseDetailsURLPath(urlPath)
	if err != nil {
		log.Errorf(ctx, "falling back to default TTL: %v", err)
		return defaultTTL
	}
	if info.requestedVersion == version.Latest {
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
		if _, ok := unitTabLookup[tab]; ok {
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
	// HTMLTitle is the value to use in the page’s <title> tag.
	HTMLTitle string

	// MetaDescription is the html used for rendering the <meta name="Description"> tag.
	MetaDescription safehtml.HTML

	// Query is the current search query (if applicable).
	Query string

	// Experiments contains the experiments currently active.
	Experiments *experiment.Set

	// DevMode indicates whether the server is running in development mode.
	DevMode bool

	// AppVersionLabel contains the current version of the app.
	AppVersionLabel string

	// GoogleTagManagerID is the ID used to load Google Tag Manager.
	GoogleTagManagerID string

	// AllowWideContent indicates whether the content should be displayed in a
	// way that’s amenable to wider viewports.
	AllowWideContent bool

	// Enables the two and three column layouts on the unit page.
	UseResponsiveLayout bool

	// UseSiteWrapper indicates whether the page content should be wrapped in the
	// Site class. This is only used for unit pages until the migration to the new
	// layout base page is completed.
	UseSiteWrapper bool
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
		s.servePage(r.Context(), w, "license-policy", page)
	})
}

// newBasePage returns a base page for the given request and title.
func (s *Server) newBasePage(r *http.Request, title string) basePage {
	return basePage{
		HTMLTitle:          title,
		Query:              searchQuery(r),
		Experiments:        experiment.FromContext(r.Context()),
		DevMode:            s.devMode,
		AppVersionLabel:    s.appVersionLabel,
		GoogleTagManagerID: s.googleTagManagerID,
	}
}

// errorPage contains fields for rendering a HTTP error page.
type errorPage struct {
	basePage
	templateName    string
	messageTemplate template.TrustedTemplate
	MessageData     interface{}
}

// PanicHandler returns an http.HandlerFunc that can be used in HTTP
// middleware. It returns an error if something goes wrong pre-rendering the
// error template.
func (s *Server) PanicHandler() (_ http.HandlerFunc, err error) {
	defer derrors.Wrap(&err, "PanicHandler")
	status := http.StatusInternalServerError
	buf, err := s.renderErrorPage(context.Background(), status, "error", nil)
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
	status       int    // HTTP status code
	responseText string // Response text to the user
	epage        *errorPage
	err          error // wrapped error
}

func (s *serverError) Error() string {
	return fmt.Sprintf("%d (%s): %v (epage=%v)", s.status, http.StatusText(s.status), s.err, s.epage)
}

func (s *serverError) Unwrap() error {
	return s.err
}

func (s *Server) errorHandler(f func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Obtain a DataSource to use for this request.
		ds := s.getDataSource(r.Context())
		if err := f(w, r, ds); err != nil {
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
		s.reportError(ctx, err, w, r)
	} else {
		log.Infof(ctx, "returning %d (%s) for error %v", serr.status, http.StatusText(serr.status), err)
	}
	if serr.responseText == "" {
		serr.responseText = http.StatusText(serr.status)
	}
	if r.Method == http.MethodPost {
		http.Error(w, serr.responseText, serr.status)
		return
	}
	s.serveErrorPage(w, r, serr.status, serr.epage)
}

// reportError sends the error to the GCP Error Reporting service.
func (s *Server) reportError(ctx context.Context, err error, w http.ResponseWriter, r *http.Request) {
	if s.reportingClient == nil {
		return
	}
	// Extract the stack trace from the error if there is one.
	var stack []byte
	if serr := (*derrors.StackError)(nil); errors.As(err, &serr) {
		stack = serr.Stack
	}
	s.reportingClient.Report(errorreporting.Entry{
		Error: err,
		Req:   r,
		Stack: stack,
	})
	log.Debugf(ctx, "reported error %v with stack size %d", err, len(stack))
	// Bypass the error-reporting middleware.
	w.Header().Set(config.BypassErrorReportingHeader, "true")
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, page *errorPage) {
	template := "error"
	if page != nil {
		if page.AppVersionLabel == "" || page.GoogleTagManagerID == "" {
			// If the basePage was properly created using newBasePage, both
			// AppVersionLabel and GoogleTagManagerID should always be set.
			page.basePage = s.newBasePage(r, "")
		}
		if page.templateName != "" {
			template = page.templateName
		}
	} else {
		page = &errorPage{
			basePage: s.newBasePage(r, ""),
		}
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
	if page.messageTemplate.String() == "" {
		page.messageTemplate = template.MakeTrustedTemplate(`<h3 class="Error-message">{{.}}</h3>`)
	}
	if page.MessageData == nil {
		page.MessageData = statusInfo
	}
	if page.HTMLTitle == "" {
		page.HTMLTitle = statusInfo
	}
	if templateName == "" {
		templateName = "error"
	}

	etmpl, err := s.findTemplate(templateName)
	if err != nil {
		return nil, err
	}
	tmpl, err := etmpl.Clone()
	if err != nil {
		return nil, err
	}
	_, err = tmpl.New("message").ParseFromTrustedTemplate(page.messageTemplate)
	if err != nil {
		return nil, err
	}

	return executeTemplate(ctx, templateName, tmpl, page)
}

// servePage is used to execute all templates for a *Server.
func (s *Server) servePage(ctx context.Context, w http.ResponseWriter, templateName string, page interface{}) {
	defer middleware.ElapsedStat(ctx, "servePage")()

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
	defer middleware.ElapsedStat(ctx, "renderPage")()

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

var templateFuncs = template.FuncMap{
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
	"stripscheme": stripScheme,
}

func stripScheme(url string) string {
	if i := strings.Index(url, "://"); i > 0 {
		return url[i+len("://"):]
	}
	return url
}

// parsePageTemplates parses html templates contained in the given base
// directory in order to generate a map of Name->*template.Template.
//
// Separate templates are used so that certain contextual functions (e.g.
// templateName) can be bound independently for each page.
//
// Templates in directories prefixed with an underscore are considered helper
// templates and parsed together with the files in each base directory.
func parsePageTemplates(base template.TrustedSource) (map[string]*template.Template, error) {
	tsc := template.TrustedSourceFromConstant
	join := template.TrustedSourceJoin

	legacyHtmlSets := [][]template.TrustedSource{
		{tsc("unit_details.tmpl"), tsc("unit.tmpl")},
		{tsc("unit_importedby.tmpl"), tsc("unit.tmpl")},
		{tsc("unit_imports.tmpl"), tsc("unit.tmpl")},
		{tsc("unit_licenses.tmpl"), tsc("unit.tmpl")},
		{tsc("unit_versions.tmpl"), tsc("unit.tmpl")},
	}

	templates := make(map[string]*template.Template)
	for _, set := range legacyHtmlSets {
		t, err := template.New("base.tmpl").Funcs(templateFuncs).ParseFilesFromTrustedSources(join(base, tsc("_base/base.tmpl")))
		if err != nil {
			return nil, fmt.Errorf("ParseFiles: %v", err)
		}
		helperGlob := join(base, tsc("html"), tsc("helpers"), tsc("*.tmpl"))
		if _, err := t.ParseGlobFromTrustedSource(helperGlob); err != nil {
			return nil, fmt.Errorf("ParseGlob(%q): %v", helperGlob, err)
		}
		header := join(base, tsc("_header"), tsc("header.tmpl"))
		footer := join(base, tsc("_footer"), tsc("footer.tmpl"))
		if _, err := t.ParseFilesFromTrustedSources(header, footer); err != nil {
			return nil, fmt.Errorf("ParseFilesFromTrustedSources(%v, %v): %v", header, footer, err)
		}

		var files []template.TrustedSource
		for _, f := range set {
			files = append(files, join(base, tsc("html"), tsc("pages"), f))
		}
		if _, err := t.ParseFilesFromTrustedSources(files...); err != nil {
			return nil, fmt.Errorf("ParseFilesFromTrustedSources(%v): %v", files, err)
		}
		templates[set[0].String()] = t
	}

	htmlSets := [][]template.TrustedSource{
		{tsc("badge")},
		{tsc("error")},
		{tsc("fetch")},
		{tsc("homepage")},
		{tsc("license-policy")},
		{tsc("search")},
		{tsc("search-help")},
		{tsc("styleguide"), tsc("unit")},
		{tsc("unit/importedby"), tsc("unit")},
		{tsc("unit/imports"), tsc("unit")},
		{tsc("unit/licenses"), tsc("unit")},
		{tsc("unit/main"), tsc("unit")},
		{tsc("unit/versions"), tsc("unit")},
	}

	for _, set := range htmlSets {
		t, err := template.New("base.tmpl").Funcs(templateFuncs).ParseFilesFromTrustedSources(join(base, tsc("_base/base.tmpl")))
		if err != nil {
			return nil, fmt.Errorf("ParseFilesFromTrustedSources: %v", err)
		}
		helperGlob := join(base, tsc("_*/*.tmpl"))
		if _, err := t.ParseGlobFromTrustedSource(helperGlob); err != nil {
			return nil, fmt.Errorf("ParseGlobFromTrustedSource(%q): %v", helperGlob, err)
		}
		var files []template.TrustedSource
		for _, f := range set {
			if _, err := t.ParseGlobFromTrustedSource(join(base, f, tsc("*.tmpl"))); err != nil {
				return nil, fmt.Errorf("ParseGlobFromTrustedSource(%v): %v", files, err)
			}
		}
		templates[set[0].String()] = t
	}

	return templates, nil
}

func (s *Server) staticHandler() http.Handler {
	staticPath := s.staticPath.String()

	// In dev mode compile TypeScript files into minified JavaScript files
	// and rebuild them on file changes.
	if s.devMode {
		ctx := context.Background()
		_, err := static.Build(static.Config{StaticPath: staticPath, Watch: true, Write: true})
		if err != nil {
			log.Error(ctx, err)
		}
	}
	return http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath)))
}
