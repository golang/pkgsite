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
	"io/fs"
	"net/http"
	hpprof "net/http/pprof"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	pagepkg "golang.org/x/pkgsite/internal/frontend/page"
	"golang.org/x/pkgsite/internal/frontend/serrors"
	"golang.org/x/pkgsite/internal/frontend/urlinfo"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/memory"
	"golang.org/x/pkgsite/internal/middleware/stats"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/pkgsite/internal/vuln"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Server can be installed to serve the go discovery frontend.
type Server struct {
	fetchServer FetchServerInterface
	// getDataSource should never be called from a handler. It is called only in Server.errorHandler.
	getDataSource      func(context.Context) internal.DataSource
	queue              queue.Queue
	templateFS         template.TrustedFS
	staticFS           fs.FS
	thirdPartyFS       fs.FS
	devMode            bool
	localMode          bool          // running locally (i.e. ./cmd/pkgsite)
	localModules       []LocalModule // locally hosted modules; empty in production
	errorPage          []byte
	appVersionLabel    string
	googleTagManagerID string
	serveStats         bool
	reporter           derrors.Reporter
	fileMux            *http.ServeMux
	vulnClient         *vuln.Client
	versionID          string
	instanceID         string
	depsDevHTTPClient  *http.Client

	mu        sync.Mutex // Protects all fields below
	templates map[string]*template.Template
}

// FetchServerInterface is an interface for the parts of the server
// that support adding packages to a queue for fetching by a worker
// server.
// TODO(matloob): rename to FetchServer once the FetchServer type moves
// to its own package
type FetchServerInterface interface {
	ServeFetch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error)
	ServePathNotFoundPage(w http.ResponseWriter, r *http.Request,
		ds internal.PostgresDB, fullPath, modulePath, requestedVersion string) (err error)
}

// ServerConfig contains everything needed by a Server.
type ServerConfig struct {
	Config *config.Config
	// Note that FetchServer may be nil.
	FetchServer FetchServerInterface
	// DataSourceGetter should return a DataSource on each call.
	// It should be goroutine-safe.
	DataSourceGetter  func(context.Context) internal.DataSource
	Queue             queue.Queue
	TemplateFS        template.TrustedFS // for loading templates safely
	StaticFS          fs.FS              // for static/ directory
	ThirdPartyFS      fs.FS              // for third_party/ directory
	DevMode           bool
	LocalMode         bool
	LocalModules      []LocalModule
	Reporter          derrors.Reporter
	VulndbClient      *vuln.Client
	DepsDevHTTPClient *http.Client
}

// NewServer creates a new Server for the given database and template directory.
func NewServer(scfg ServerConfig) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(...)")
	ts, err := parsePageTemplates(scfg.TemplateFS)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %v", err)
	}
	dochtml.LoadTemplates(scfg.TemplateFS)
	s := &Server{
		fetchServer:       scfg.FetchServer,
		getDataSource:     scfg.DataSourceGetter,
		queue:             scfg.Queue,
		templateFS:        scfg.TemplateFS,
		staticFS:          scfg.StaticFS,
		thirdPartyFS:      scfg.ThirdPartyFS,
		devMode:           scfg.DevMode,
		localMode:         scfg.LocalMode,
		localModules:      scfg.LocalModules,
		templates:         ts,
		reporter:          scfg.Reporter,
		fileMux:           http.NewServeMux(),
		vulnClient:        scfg.VulndbClient,
		depsDevHTTPClient: scfg.DepsDevHTTPClient,
	}
	if s.depsDevHTTPClient == nil {
		s.depsDevHTTPClient = http.DefaultClient
	}
	if scfg.Config != nil {
		s.appVersionLabel = scfg.Config.AppVersionLabel()
		s.googleTagManagerID = scfg.Config.GoogleTagManagerID
		s.serveStats = scfg.Config.ServeStats
		s.versionID = scfg.Config.VersionID
		s.instanceID = scfg.Config.InstanceID
	}
	errorPageBytes, err := s.renderErrorPage(context.Background(), http.StatusInternalServerError, "error", nil)
	if err != nil {
		return nil, fmt.Errorf("s.renderErrorPage(http.StatusInternalServerError, nil): %v", err)
	}
	s.errorPage = errorPageBytes
	return s, nil
}

// A Cacher is used to create request caches for http handlers.
type Cacher interface {
	// Cache returns a new middleware that caches every request.
	// The name of the cache is used only for metrics.
	// The expirer is a func that is used to map a new request to its TTL.
	// authHeader is the header key used by the cache to know that a
	// request should bypass the cache.
	// authValues is the set of values that could be set on the authHeader in
	// order to bypass the cache.
	Cache(name string, expirer func(r *http.Request) time.Duration, authValues []string) func(http.Handler) http.Handler
}

// Install registers server routes using the given handler registration func.
// authValues is the set of values that can be set on authHeader to bypass the
// cache.
func (s *Server) Install(handle func(string, http.Handler), cacher Cacher, authValues []string) {
	var (
		detailHandler http.Handler = s.errorHandler(s.serveDetails)
		fetchHandler  http.Handler
		searchHandler http.Handler = s.errorHandler(s.serveSearch)
		vulnHandler   http.Handler = s.errorHandler(s.serveVuln)
	)
	if s.fetchServer != nil {
		fetchHandler = s.errorHandler(s.fetchServer.ServeFetch)
	}
	if cacher != nil {
		// The cache middleware uses the URL string as the key for content served
		// by the handlers it wraps. Be careful not to wrap the handler it returns
		// with a handler that rewrites the URL in a way that could cause key
		// collisions, like http.StripPrefix.
		detailHandler = cacher.Cache("details", detailsTTL, authValues)(detailHandler)
		searchHandler = cacher.Cache("search", searchTTL, authValues)(searchHandler)
		vulnHandler = cacher.Cache("vuln", vulnTTL, authValues)(vulnHandler)
	}
	// Each AppEngine instance is created in response to a start request, which
	// is an empty HTTP GET request to /_ah/start when scaling is set to manual
	// or basic, and /_ah/warmup when scaling is automatic and min_instances is
	// set. AppEngine sends this request to bring an instance into existence.
	// See details for /_ah/start at
	// https://cloud.google.com/appengine/docs/standard/go/how-instances-are-managed#startup
	// and for /_ah/warmup at
	// https://cloud.google.com/appengine/docs/standard/go/configuring-warmup-requests.
	handle("/_ah/", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		log.Infof(r.Context(), "Request made to %q", r.URL.Path)
	}))
	handle("/static/", s.staticHandler())
	handle("/third_party/", http.StripPrefix("/third_party", http.FileServer(http.FS(s.thirdPartyFS))))
	handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveFileFS(w, r, s.staticFS, "shared/icon/favicon.ico")
	}))

	handle("/sitemap/", http.StripPrefix("/sitemap/", http.FileServer(http.Dir("private/sitemap"))))
	handle("/mod/", http.HandlerFunc(s.handleModuleDetailsRedirect))
	handle("/pkg/", http.HandlerFunc(s.handlePackageDetailsRedirect))
	if fetchHandler != nil {
		handle("/fetch/", fetchHandler)
	}
	handle("/play/compile", http.HandlerFunc(s.proxyPlayground))
	handle("/play/fmt", http.HandlerFunc(s.handleFmt))
	handle("/play/share", http.HandlerFunc(s.proxyPlayground))
	handle("/search", searchHandler)
	handle("/search-help", s.staticPageHandler("search-help", "Search Help"))
	handle("/license-policy", s.licensePolicyHandler())
	handle("/about", s.staticPageHandler("about", "About"))
	handle("/badge/", http.HandlerFunc(s.badgeHandler))
	handle("/styleguide", http.HandlerFunc(s.errorHandler(s.serveStyleGuide)))
	handle("/C", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Package "C" is a special case: redirect to /cmd/cgo.
		// (This is what golang.org/C does.)
		http.Redirect(w, r, "/cmd/cgo", http.StatusMovedPermanently)
	}))
	handle("/golang.org/x", s.staticPageHandler("subrepo", "Sub-repositories"))
	handle("/files/", http.StripPrefix("/files", s.fileMux))
	handle("/vuln/", vulnHandler)
	handle("/opensearch.xml", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveFileFS(w, r, s.staticFS, "shared/opensearch.xml")
	}))
	handle("/", detailHandler)
	if s.serveStats {
		handle("/detail-stats/",
			stats.Stats()(http.StripPrefix("/detail-stats", s.errorHandler(s.serveDetails))))
		handle("/search-stats/",
			stats.Stats()(http.StripPrefix("/search-stats", s.errorHandler(s.serveSearch))))
	}
	handle("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(`User-agent: *
Disallow: /search?*
Disallow: /fetch/*
Sitemap: https://pkg.go.dev/sitemap/index.xml
`))
	}))
	s.installDebugHandlers(handle)
}

// installDebugHandlers installs handlers for debugging. Most of the handlers
// are provided by the net/http/pprof package. Although that package installs
// them on the default ServeMux in its init function, we must install them on
// our own ServeMux.
func (s *Server) installDebugHandlers(handle func(string, http.Handler)) {

	ifDebug := func(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dbg := r.Header.Get(config.AllowDebugHeader)
			if dbg == "" || dbg != os.Getenv("GO_DISCOVERY_DEBUG_HEADER_VALUE") {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			h(w, r)
		})
	}

	handle("/_debug/pprof/", ifDebug(hpprof.Index))
	handle("/_debug/pprof/cmdline", ifDebug(hpprof.Cmdline))
	handle("/_debug/pprof/profile", ifDebug(hpprof.Profile))
	handle("/_debug/pprof/symbol", ifDebug(hpprof.Symbol))
	handle("/_debug/pprof/trace", ifDebug(hpprof.Trace))

	handle("/_debug/info", ifDebug(func(w http.ResponseWriter, _ *http.Request) {
		row := func(a, b string) {
			fmt.Fprintf(w, "<tr><td>%s</td> <td>%s</td></tr>\n", a, b)
		}
		memrow := func(s string, m uint64) {
			fmt.Fprintf(w, "<tr><td>%s</td> <td align='right'>%s</td></tr>\n", s, memory.Format(m))
		}

		fmt.Fprintf(w, "<html><body style='font-family: sans-serif'>\n")

		fmt.Fprintf(w, "<table>\n")
		row("Service", os.Getenv("K_SERVICE"))
		row("Config", os.Getenv("K_CONFIGURATION"))
		row("Revision", s.versionID)
		row("Instance", s.instanceID)
		fmt.Fprintf(w, "</table>\n")

		gm := memory.ReadRuntimeStats()
		pm, err := memory.ReadProcessStats()
		if err != nil {
			http.Error(w, fmt.Sprintf("reading process stats: %v", err), http.StatusInternalServerError)
			return
		}
		sm, err := memory.ReadSystemStats()
		if err != nil {
			http.Error(w, fmt.Sprintf("reading system stats: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "<table>\n")
		memrow("Go Alloc", gm.Alloc)
		memrow("Go Sys", gm.Sys)
		memrow("Process VSize", pm.VSize)
		memrow("Process RSS", pm.RSS)
		memrow("System Total", sm.Total)
		memrow("System Used", sm.Used)
		cm, err := memory.ReadCgroupStats()
		if err != nil {
			row("CGroup Stats", "unavailable")
		} else {
			for k, v := range cm {
				memrow("CGroup "+k, v)
			}
		}
		fmt.Fprintf(w, "</table>\n")

		fmt.Fprintf(w, "</body></html>\n")

	}))
}

// InstallFS adds path under the /files handler, serving the files in fsys.
func (s *Server) InstallFS(path string, fsys fs.FS) {
	s.fileMux.Handle(path+"/", http.StripPrefix(path, http.FileServer(http.FS(fsys))))
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
	// symbolSearchTTL is used for most symbol searches.
	symbolSearchTTL = 24 * time.Hour
	// slowSymbolSearchTTL is for symbol searches that are known to be slow.
	slowSymbolSearchTTL = 14 * 24 * time.Hour
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
	info, err := urlinfo.ParseDetailsURLPath(urlPath)
	if err != nil {
		log.Errorf(ctx, "falling back to default TTL: %v", err)
		return defaultTTL
	}
	if info.RequestedVersion == version.Latest {
		return shortTTL
	}
	if tab == "importedby" || tab == "versions" {
		return defaultTTL
	}
	return longTTL
}

var slowSymbolSearches = map[string]bool{
	"new":                            true,
	"version":                        true,
	"config":                         true,
	"client":                         true,
	"useragent":                      true,
	"baseclient":                     true,
	"error":                          true,
	"defaultbaseuri":                 true,
	"newwithbaseuri":                 true,
	"resource":                       true,
	"newclient":                      true,
	"get":                            true,
	"operationsclient":               true,
	"newoperationsclient":            true,
	"newoperationsclientwithbaseuri": true,
	"operation":                      true,
	"operationsclientapi":            true,
	"baseclient.baseuri":             true,
	"failed":                         true,
	"baseclient.subscriptionid":      true,
}

// searchTTL assigns the cache TTL for search requests.
func searchTTL(r *http.Request) time.Duration {
	if searchMode(r) == searchModeSymbol {
		q, _ := searchQueryAndFilters(r)
		if slowSymbolSearches[strings.ToLower(q)] {
			// Slow searches should be computed on deploy. Cache them for a long time.
			return slowSymbolSearchTTL
		}
	}
	return symbolSearchTTL
}

// vulnTTL assigns the cache TTL for vuln requests.
func vulnTTL(r *http.Request) time.Duration {
	return defaultTTL
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
	if tag == "search" {
		tag += "-" + searchMode(r)
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

// licensePolicyPage is used to generate the static license policy page.
type licensePolicyPage struct {
	pagepkg.BasePage
	LicenseFileNames []string
	LicenseTypes     []licenses.AcceptedLicenseInfo
}

func (s *Server) licensePolicyHandler() http.HandlerFunc {
	lics := licenses.AcceptedLicenses()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := licensePolicyPage{
			BasePage:         s.newBasePage(r, "License Policy"),
			LicenseFileNames: licenses.FileNames,
			LicenseTypes:     lics,
		}
		s.servePage(r.Context(), w, "license-policy", page)
	})
}

// newBasePage returns a base page for the given request and title.
func (s *Server) newBasePage(r *http.Request, title string) pagepkg.BasePage {
	q := rawSearchQuery(r)

	var searchPrompt string
	if s.localMode {
		// Symbol search is not supported in local mode.
		searchPrompt = "Search packages"
	} else {
		searchPrompt = "Search packages or symbols"
	}

	return pagepkg.BasePage{
		HTMLTitle:          title,
		Query:              q,
		Experiments:        experiment.FromContext(r.Context()),
		DevMode:            s.devMode,
		LocalMode:          s.localMode,
		AppVersionLabel:    s.appVersionLabel,
		GoogleTagManagerID: s.googleTagManagerID,
		SearchPrompt:       searchPrompt,
		SearchModePackage:  searchModePackage,
		SearchModeSymbol:   searchModeSymbol,
		// By default, the SearchMode is set to the empty string, which
		// indicates that we should use heuristics to determine whether the
		// user wants to search for symbols or packages.
		SearchMode: "",
	}
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
	var serr *serrors.ServerError
	if !errors.As(err, &serr) {
		serr = &serrors.ServerError{Status: http.StatusInternalServerError, Err: err}
	}
	if serr.Status == http.StatusInternalServerError {
		log.Error(ctx, err)
		s.reportError(ctx, err, w, r)
	} else {
		log.Infof(ctx, "returning %d (%s) for error %v", serr.Status, http.StatusText(serr.Status), err)
	}
	if serr.ResponseText == "" {
		serr.ResponseText = http.StatusText(serr.Status)
	}
	if r.Method == http.MethodPost {
		http.Error(w, serr.ResponseText, serr.Status)
		return
	}
	s.serveErrorPage(w, r, serr.Status, serr.Epage)
}

// reportError sends the error to the GCP Error Reporting service.
func (s *Server) reportError(ctx context.Context, err error, w http.ResponseWriter, r *http.Request) {
	if s.reporter == nil {
		return
	}
	// Extract the stack trace from the error if there is one.
	var stack []byte
	if serr := (*derrors.StackError)(nil); errors.As(err, &serr) {
		stack = serr.Stack
	}
	s.reporter.Report(err, r, stack)
	log.Debugf(ctx, "reported error %v with stack size %d", err, len(stack))
	// Bypass the error-reporting middleware.
	w.Header().Set(config.BypassErrorReportingHeader, "true")
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, page *pagepkg.ErrorPage) {
	template := "error"
	if page != nil {
		if page.AppVersionLabel == "" || page.GoogleTagManagerID == "" {
			// If the BasePage was properly created using newBasePage, both
			// AppVersionLabel and GoogleTagManagerID should always be set.
			page.BasePage = s.newBasePage(r, "")
		}
		if page.TemplateName != "" {
			template = page.TemplateName
		}
	} else {
		page = &pagepkg.ErrorPage{
			BasePage: s.newBasePage(r, ""),
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

// renderErrorPage executes error.tmpl with the given ErrorPage
func (s *Server) renderErrorPage(ctx context.Context, status int, templateName string, page *pagepkg.ErrorPage) ([]byte, error) {
	statusInfo := fmt.Sprintf("%d %s", status, http.StatusText(status))
	if page == nil {
		page = &pagepkg.ErrorPage{}
	}
	if page.MessageTemplate.String() == "" {
		page.MessageTemplate = template.MakeTrustedTemplate(`<h3 class="Error-message">{{.}}</h3>`)
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
	_, err = tmpl.New("message").ParseFromTrustedTemplate(page.MessageTemplate)
	if err != nil {
		return nil, err
	}

	return executeTemplate(ctx, templateName, tmpl, page)
}

// servePage is used to execute all templates for a *Server.
func (s *Server) servePage(ctx context.Context, w http.ResponseWriter, templateName string, page any) {
	defer stats.Elapsed(ctx, "servePage")()

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
func (s *Server) renderPage(ctx context.Context, templateName string, page any) ([]byte, error) {
	defer stats.Elapsed(ctx, "renderPage")()

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
		s.templates, err = parsePageTemplates(s.templateFS)
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

func executeTemplate(ctx context.Context, templateName string, tmpl *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Errorf(ctx, "Error executing page template %q: %v", templateName, err)
		return nil, err
	}
	return buf.Bytes(), nil
}

var templateFuncs = template.FuncMap{
	"add":      func(i, j int) int { return i + j },
	"subtract": func(i, j int) int { return i - j },
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
	"capitalize":  cases.Title(language.Und).String,
	"queryescape": url.QueryEscape,
}

func stripScheme(url string) string {
	if i := strings.Index(url, "://"); i > 0 {
		return url[i+len("://"):]
	}
	return url
}

// parsePageTemplates parses html templates contained in the given filesystem in
// order to generate a map of Name->*template.Template.
//
// Separate templates are used so that certain contextual functions (e.g.
// templateName) can be bound independently for each page.
//
// Templates in directories prefixed with an underscore are considered helper
// templates and parsed together with the files in each base directory.
func parsePageTemplates(fsys template.TrustedFS) (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)
	htmlSets := [][]string{
		{"about"},
		{"badge"},
		{"error"},
		{"fetch"},
		{"homepage"},
		{"license-policy"},
		{"search"},
		{"search-help"},
		{"styleguide"},
		{"subrepo"},
		{"unit/importedby", "unit"},
		{"unit/imports", "unit"},
		{"unit/licenses", "unit"},
		{"unit/main", "unit"},
		{"unit/versions", "unit"},
		{"vuln"},
		{"vuln/main", "vuln"},
		{"vuln/list", "vuln"},
		{"vuln/entry", "vuln"},
	}

	for _, set := range htmlSets {
		t, err := template.New("frontend.tmpl").Funcs(templateFuncs).ParseFS(fsys, "frontend/*.tmpl")
		if err != nil {
			return nil, fmt.Errorf("ParseFS: %v", err)
		}
		helperGlob := "shared/*/*.tmpl"
		if _, err := t.ParseFS(fsys, helperGlob); err != nil {
			return nil, fmt.Errorf("ParseFS(%q): %v", helperGlob, err)
		}
		for _, f := range set {
			if _, err := t.ParseFS(fsys, path.Join("frontend", f, "*.tmpl")); err != nil {
				return nil, fmt.Errorf("ParseFS(%v): %v", f, err)
			}
		}
		templates[set[0]] = t
	}

	return templates, nil
}

func (s *Server) staticHandler() http.Handler {
	return http.StripPrefix("/static/", http.FileServer(http.FS(s.staticFS)))
}

// serveFileFS serves a file from the given filesystem.
func serveFileFS(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	fs := http.FileServer(http.FS(fsys))
	r.URL.Path = name
	fs.ServeHTTP(w, r)
}
