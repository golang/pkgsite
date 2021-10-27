// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package worker provides functionality for running a worker service.
// Its primary operation is to fetch modules from a proxy and write them to the
// database.
package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/errorreporting"
	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/cache"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/poller"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// Server can be installed to serve the go discovery worker.
type Server struct {
	cfg             *config.Config
	indexClient     *index.Client
	proxyClient     *proxy.Client
	sourceClient    *source.Client
	cache           *cache.Cache
	db              *postgres.DB
	queue           queue.Queue
	reportingClient *errorreporting.Client
	templates       map[string]*template.Template
	staticPath      template.TrustedSource
	getExperiments  func() []*internal.Experiment
	workerDBInfo    func() *postgres.UserInfo
	loadShedder     *loadShedder
}

// ServerConfig contains everything needed by a Server.
type ServerConfig struct {
	DB               *postgres.DB
	IndexClient      *index.Client
	ProxyClient      *proxy.Client
	SourceClient     *source.Client
	RedisCacheClient *redis.Client
	Queue            queue.Queue
	ReportingClient  *errorreporting.Client
	StaticPath       template.TrustedSource
	GetExperiments   func() []*internal.Experiment
}

const (
	indexTemplate    = "index.tmpl"
	versionsTemplate = "versions.tmpl"
)

// NewServer creates a new Server with the given dependencies.
func NewServer(cfg *config.Config, scfg ServerConfig) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(db, %+v)", scfg)
	t1, err := parseTemplate(scfg.StaticPath, template.TrustedSourceFromConstant(indexTemplate))
	if err != nil {
		return nil, err
	}
	t2, err := parseTemplate(scfg.StaticPath, template.TrustedSourceFromConstant(versionsTemplate))
	if err != nil {
		return nil, err
	}
	ts := template.TrustedSourceJoin(scfg.StaticPath, template.TrustedSourceFromConstant("doc"))
	tfs := template.TrustedFSFromTrustedSource(ts)
	dochtml.LoadTemplates(tfs)
	templates := map[string]*template.Template{
		indexTemplate:    t1,
		versionsTemplate: t2,
	}
	var c *cache.Cache
	if scfg.RedisCacheClient != nil {
		c = cache.New(scfg.RedisCacheClient)
	}

	// Update information about DB locks, etc. every few seconds.
	p := poller.New(&postgres.UserInfo{}, func(ctx context.Context) (interface{}, error) {
		return scfg.DB.GetUserInfo(ctx, "worker")
	}, func(err error) { log.Error(context.Background(), err) })
	p.Start(context.Background(), 10*time.Second)

	s := &Server{
		cfg:             cfg,
		db:              scfg.DB,
		indexClient:     scfg.IndexClient,
		proxyClient:     scfg.ProxyClient,
		sourceClient:    scfg.SourceClient,
		cache:           c,
		queue:           scfg.Queue,
		reportingClient: scfg.ReportingClient,
		templates:       templates,
		staticPath:      scfg.StaticPath,
		getExperiments:  scfg.GetExperiments,
		workerDBInfo:    func() *postgres.UserInfo { return p.Current().(*postgres.UserInfo) },
	}
	s.setLoadShedder(context.Background())
	return s, nil
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler)) {
	// rmw wires in error reporting to the handler. It is configured here, in
	// Install, because not every handler should have error reporting.
	rmw := middleware.Identity()
	if s.reportingClient != nil {
		rmw = middleware.ErrorReporting(s.reportingClient.Report)
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

	// scheduled: poll polls the Module Index for new modules
	// that have been published and inserts that metadata into
	// module_version_states.
	// This endpoint is intended to be invoked periodically by a scheduler.
	// See the note about duplicate tasks for "/enqueue" below.
	handle("/poll", rmw(s.errorHandler(s.handlePollIndex)))

	// scheduled: update-imported-by-count update the imported_by_count for
	// packages in search_documents where imported_by_count_updated_at is null
	// or imported_by_count_updated_at < version_updated_at.
	// This endpoint is intended to be invoked periodically by a scheduler.
	handle("/update-imported-by-count", rmw(s.errorHandler(s.handleUpdateImportedByCount)))

	// task-queue: fetch fetches a module version from the Module Mirror, and
	// processes the contents, and inserts it into the database. If a fetch
	// request fails for any reason other than an http.StatusInternalServerError,
	// it will return an http.StatusOK so that the task queue does not retry
	// fetching module versions that have a terminal error.
	// This endpoint is intended to be invoked by a task queue with semantics like
	// Google Cloud Task Queues.
	handle("/fetch/", http.StripPrefix("/fetch", rmw(http.HandlerFunc(s.handleFetch))))

	// scheduled: fetch-std-master checks if the std@master version in the
	// database is up to date with the version at HEAD. If not, a fetch request
	// is queued to refresh the std@master version.
	handle("/fetch-std-master", rmw(s.errorHandler(s.handleFetchStdSupportedBranches)))

	// scheduled: enqueue queries the module_version_states table for the next
	// batch of module versions to process, and enqueues them for processing.
	// Normally this will not cause duplicate processing, because Cloud Tasks
	// are de-duplicated. That does not apply after a task has been finished or
	// deleted for Server.taskIDChangeInterval (see
	// https://cloud.google.com/tasks/docs/reference/rpc/google.cloud.tasks.v2#createtaskrequest,
	// under "Task De-duplication"). If you cannot wait, you can force
	// duplicate tasks by providing any string as the "suffix" query parameter.
	handle("/enqueue", rmw(s.errorHandler(s.handleEnqueue)))

	// TODO: remove after /queue is in production and the scheduler jobs have been changed.
	// scheduled: requeue queries the module_version_states table for the next
	// batch of module versions to process, and enqueues them for processing.
	// Normally this will not cause duplicate processing, because Cloud Tasks
	// are de-duplicated. That does not apply after a task has been finished or
	// deleted for about an hour
	// https://cloud.google.com/tasks/docs/reference/rpc/google.cloud.tasks.v2#createtaskrequest,
	// under "Task De-duplication"). If you cannot wait, you can force
	// duplicate tasks by providing any string as the "suffix" query parameter.
	handle("/requeue", rmw(s.errorHandler(s.handleEnqueue)))

	// manual: reprocess sets a reprocess status for all records in the
	// module_version_states table that were processed by an app_version that
	// occurred after the provided app_version param, so that they will be
	// scheduled for reprocessing the next time a request to /enqueue is made.
	// If a status param is provided only module versions with that status will
	// be reprocessed.
	handle("/reprocess", rmw(s.errorHandler(s.handleReprocess)))

	// manual: populate-stdlib inserts all modules of the Go standard
	// library into the tasks queue to be processed and inserted into the
	// database. handlePopulateStdLib should be updated whenever a new
	// version of Go is released.
	// see the comments on duplicate tasks for "/requeue", above.
	handle("/populate-stdlib", rmw(s.errorHandler(s.handlePopulateStdLib)))

	// manual: populate-search-documents repopulates every row in the
	// search_documents table that was last updated before the time in the
	// "before" query parameter.
	handle("/repopulate-search-documents", rmw(s.errorHandler(s.handleRepopulateSearchDocuments)))

	// manual: clear-cache clears the redis cache.
	handle("/clear-cache", rmw(s.errorHandler(s.clearCache)))

	// manual: delete the specified module version.
	handle("/delete/", http.StripPrefix("/delete", rmw(s.errorHandler(s.handleDelete))))

	// scheduled ("limit" query param): clean some eligible module versions selected from the DB
	// manual ("module" query param): clean all versions of a given module.
	handle("/clean", rmw(s.errorHandler(s.handleClean)))

	handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticPath.String()))))

	// returns an HTML page displaying information about recent versions that were processed.
	handle("/versions", http.HandlerFunc(s.handleHTMLPage(s.doVersionsPage)))

	// Health check.
	handle("/healthz", http.HandlerFunc(s.handleHealthCheck))

	handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/worker/favicon.ico")
	}))

	// returns an HTML page displaying the homepage.
	handle("/", http.HandlerFunc(s.handleHTMLPage(s.doIndexPage)))
}

// handleUpdateImportedByCount updates imported_by_count for all packages.
func (s *Server) handleUpdateImportedByCount(w http.ResponseWriter, r *http.Request) error {
	n, err := s.db.UpdateSearchDocumentsImportedByCount(r.Context())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "updated %d packages", n)
	return nil
}

// handleRepopulateSearchDocuments repopulates every row in the search_documents table
// that was last updated before the given time.
func (s *Server) handleRepopulateSearchDocuments(w http.ResponseWriter, r *http.Request) error {
	limit := parseLimitParam(r, 100)
	beforeParam := r.FormValue("before")
	if beforeParam == "" {
		return &serverError{
			http.StatusBadRequest,
			errors.New("must provide 'before' query param as an RFC3339 datetime"),
		}
	}
	before, err := time.Parse(time.RFC3339, beforeParam)
	if err != nil {
		return &serverError{http.StatusBadRequest, err}
	}

	ctx := r.Context()
	log.Infof(ctx, "Repopulating search documents for %d packages", limit)
	sdargs, err := s.db.GetPackagesForSearchDocumentUpsert(ctx, before, limit)
	if err != nil {
		return err
	}

	for _, args := range sdargs {
		if err := postgres.UpsertSearchDocument(ctx, s.db.Underlying(), args); err != nil {
			return err
		}
	}
	return nil
}

// handleFetch executes a fetch request and returns a http.StatusOK if the
// status is not http.StatusInternalServerError, so that the task queue does
// not retry fetching module versions that have a terminal error.
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h1>Hello, Go Discovery Fetch Service!</h1>")
		fmt.Fprintf(w, `<p><a href="/fetch/rsc.io/quote/@v/v1.0.0">Fetch an example module</a></p>`)
		return
	}
	msg, code := s.doFetch(w, r)
	if code == http.StatusInternalServerError || code == http.StatusServiceUnavailable {
		log.Infof(r.Context(), "doFetch of %s returned %d; returning that code to retry task", r.URL.Path, code)
		http.Error(w, http.StatusText(code), code)
		return
	}
	if code/100 != 2 {
		log.Infof(r.Context(), "doFetch of %s returned code %d; returning OK to avoid retry", r.URL.Path, code)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if code/100 == 2 {
		log.Info(r.Context(), msg)
		fmt.Fprintln(w, msg)
	}
	fmt.Fprintln(w, http.StatusText(code))
}

// doFetch executes a fetch request and returns the msg and status.
func (s *Server) doFetch(w http.ResponseWriter, r *http.Request) (string, int) {
	ctx := r.Context()
	modulePath, requestedVersion, err := parseModulePathAndVersion(r.URL.Path)
	if err != nil {
		return err.Error(), http.StatusBadRequest
	}

	f := &Fetcher{
		ProxyClient:  s.proxyClient.WithCache(),
		SourceClient: s.sourceClient,
		DB:           s.db,
		Cache:        s.cache,
		loadShedder:  s.loadShedder,
	}
	if r.FormValue(queue.DisableProxyFetchParam) == queue.DisableProxyFetchValue {
		f.ProxyClient = f.ProxyClient.WithFetchDisabled()
	}
	if r.FormValue(queue.SourceParam) == queue.SourceFrontendValue {
		f.Source = queue.SourceFrontendValue
	}
	code, resolvedVersion, err := f.FetchAndUpdateState(ctx, modulePath, requestedVersion, s.cfg.AppVersionLabel())
	if code == http.StatusInternalServerError {
		s.reportError(ctx, err, w, r)
		return err.Error(), code
	}
	return fmt.Sprintf("fetched and updated %s@%s", modulePath, resolvedVersion), code
}

// reportError sends the error to the GCP Error Reporting service.
// TODO(jba): factor out from here and frontend/server.go.
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

// parseModulePathAndVersion returns the module and version specified by p. p
// is assumed to have either of the following two structures:
//   - <module>/@v/<version>
//   - <module>/@latest
// (this is symmetric with the proxy url scheme)
func parseModulePathAndVersion(requestPath string) (string, string, error) {
	p := strings.TrimPrefix(requestPath, "/")
	if strings.HasSuffix(p, "/@latest") {
		modulePath := strings.TrimSuffix(p, "/@latest")
		if modulePath == "" {
			return "", "", fmt.Errorf("invalid module path: %q", modulePath)
		}
		return modulePath, version.Latest, nil
	}
	var parts []string
	if strings.Contains(requestPath, "/@v") {
		parts = strings.Split(p, "/@v/")
	} else {
		parts = strings.Split(p, "@v")
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path: %q", requestPath)
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid path: %q", requestPath)
	}
	return parts[0], parts[1], nil
}

func (s *Server) handlePollIndex(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "handlePollIndex(%q)", r.URL.Path)
	ctx := r.Context()
	limit := parseLimitParam(r, 10)
	since, err := s.db.LatestIndexTimestamp(ctx)
	if err != nil {
		return err
	}
	modules, err := s.indexClient.GetVersions(ctx, since, limit)
	if err != nil {
		return err
	}
	if err := s.db.InsertIndexVersions(ctx, modules); err != nil {
		return err
	}
	log.Infof(ctx, "Inserted %d modules from the index", len(modules))
	s.computeProcessingLag(ctx)
	s.computeUnprocessedModules(ctx)
	recordWorkerDBInfo(ctx, s.workerDBInfo())
	return nil
}

func (s *Server) computeProcessingLag(ctx context.Context) {
	ot, err := s.db.StalenessTimestamp(ctx)
	if errors.Is(err, derrors.NotFound) {
		recordProcessingLag(ctx, 0)
	} else if err != nil {
		log.Warningf(ctx, "StalenessTimestamp: %v", err)
		return
	} else {
		// If the times on this machine and the machine that wrote the index
		// timestamp into the DB are out of sync, then the difference we compute
		// here will be off. But that is unlikely since both machines are
		// running on GCP.
		recordProcessingLag(ctx, time.Since(ot))
	}
}

func (s *Server) computeUnprocessedModules(ctx context.Context) {
	total, new, err := s.db.NumUnprocessedModules(ctx)
	if err != nil {
		log.Warningf(ctx, "%v", err)
		return
	}
	recordUnprocessedModules(ctx, total, new)
}

// handleEnqueue queries the module_version_states table for the next batch of
// module versions to process, and enqueues them for processing. Note that this
// may cause duplicate processing.
func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "handleEnqueue(%q)", r.URL.Path)
	ctx := r.Context()
	limit := parseLimitParam(r, 10)
	suffixParam := r.FormValue("suffix") // append to task name to avoid deduplication
	span := trace.FromContext(r.Context())
	span.Annotate([]trace.Attribute{trace.Int64Attribute("limit", int64(limit))}, "processed limit")
	modules, err := s.db.GetNextModulesToFetch(ctx, limit)
	if err != nil {
		return err
	}

	span.Annotate([]trace.Attribute{trace.Int64Attribute("modules to fetch", int64(len(modules)))}, "processed limit")
	w.Header().Set("Content-Type", "text/plain")
	log.Infof(ctx, "Scheduling modules to be fetched: queuing %d modules", len(modules))

	// Enqueue concurrently, because sequentially takes a while.
	const concurrentEnqueues = 10
	var (
		mu                 sync.Mutex
		nEnqueued, nErrors int
	)
	sem := make(chan struct{}, concurrentEnqueues)
	for _, m := range modules {
		m := m
		opts := queue.Options{
			Suffix:            suffixParam,
			DisableProxyFetch: shouldDisableProxyFetch(m),
			Source:            queue.SourceWorkerValue,
		}
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			enqueued, err := s.queue.ScheduleFetch(ctx, m.ModulePath, m.Version, &opts)
			mu.Lock()
			if err != nil {
				log.Errorf(ctx, "enqueuing: %v", err)
				nErrors++
			} else if enqueued {
				nEnqueued++
				recordEnqueue(r.Context(), m.Status)
			}
			mu.Unlock()
		}()
	}
	// Wait for goroutines to finish.
	for i := 0; i < concurrentEnqueues; i++ {
		sem <- struct{}{}
	}
	log.Infof(ctx, "Successfully scheduled modules to be fetched: %d modules enqueued, %d errors", nEnqueued, nErrors)
	return nil
}

func shouldDisableProxyFetch(m *internal.ModuleVersionState) bool {
	// Don't ask the proxy to fetch if this module is being reprocessed.
	// We use codes 52x and 54x for reprocessing.
	return m.Status/10 == 52 || m.Status/10 == 54
}

// handleHTMLPage returns an HTML page using a template from s.templates.
func (s *Server) handleHTMLPage(f func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			log.Errorf(r.Context(), "handleHTMLPage", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

func (s *Server) handleFetchStdSupportedBranches(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "handleFetchStdSupportedBranches")
	resolvedHashes, err := stdlib.ResolveSupportedBranches()
	if err != nil {
		return err
	}
	for requestedVersion := range stdlib.SupportedBranches {
		var schedule bool
		resolvedHash := resolvedHashes[requestedVersion]
		vm, err := s.db.GetVersionMap(r.Context(), stdlib.ModulePath, requestedVersion)
		switch {
		case err == nil:
			schedule = !stdlib.VersionMatchesHash(vm.ResolvedVersion, resolvedHash)
			log.Debugf(r.Context(), "stdlib branch %s: have %s, remote is %q; scheduling = %t",
				requestedVersion, vm.ResolvedVersion, resolvedHash, schedule)
		case errors.Is(err, derrors.NotFound):
			schedule = true
		default:
			return err
		}
		if schedule {
			if _, err := s.queue.ScheduleFetch(r.Context(), stdlib.ModulePath, requestedVersion, nil); err != nil {
				return fmt.Errorf("error scheduling fetch for %s: %w", requestedVersion, err)
			}
		}
	}
	return nil
}

func (s *Server) handlePopulateStdLib(w http.ResponseWriter, r *http.Request) error {
	msg, err := s.doPopulateStdLib(r.Context(), r.FormValue("suffix"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil {
		return fmt.Errorf("handlePopulateStdLib: %v", err)
	}
	log.Infof(r.Context(), "handlePopulateStdLib: %s", msg)
	_, _ = io.WriteString(w, msg)
	return nil
}

func (s *Server) doPopulateStdLib(ctx context.Context, suffix string) (string, error) {
	versions, err := stdlib.Versions()
	if err != nil {
		return "", err
	}
	for _, v := range versions {
		opts := &queue.Options{
			Suffix: suffix,
		}
		if _, err := s.queue.ScheduleFetch(ctx, stdlib.ModulePath, v, opts); err != nil {
			return "", fmt.Errorf("error scheduling fetch for %s: %w", v, err)
		}
	}
	return fmt.Sprintf("Scheduling modules to be fetched: %s.\n", strings.Join(versions, ", ")), nil
}

func (s *Server) handleReprocess(w http.ResponseWriter, r *http.Request) error {
	appVersion := r.FormValue("app_version")
	if appVersion == "" {
		return &serverError{http.StatusBadRequest, errors.New("app_version was not specified")}
	}
	if err := config.ValidateAppVersion(appVersion); err != nil {
		return &serverError{http.StatusBadRequest, fmt.Errorf("config.ValidateAppVersion(%q): %v", appVersion, err)}
	}

	// Reprocess only the latest version of a module version with a previous
	// status of 200 or 290.
	latestOnly := r.FormValue("latest_only") == "true"
	if latestOnly {
		if err := s.db.UpdateModuleVersionStatesForReprocessingLatestOnly(r.Context(), appVersion); err != nil {
			return err
		}
		fmt.Fprintf(w, "Scheduled latest version of modules to be reprocessed for appVersion > %q.", appVersion)
		return nil
	}
	searchDocuments := r.FormValue("search_documents") == "true"
	if searchDocuments {
		if err := s.db.UpdateModuleVersionStatesForReprocessingSearchDocumentsOnly(r.Context(), appVersion); err != nil {
			return err
		}
		fmt.Fprintf(w, "Scheduled modules in search_documents to be reprocessed for appVersion > %q.", appVersion)
		return nil
	}

	// Reprocess only module versions with the given status code.
	status := r.FormValue("status")
	if status != "" {
		code, err := strconv.Atoi(status)
		if err != nil {
			return &serverError{http.StatusBadRequest, fmt.Errorf("status is invalid: %q", status)}
		}
		if err := s.db.UpdateModuleVersionStatesWithStatus(r.Context(), code, appVersion); err != nil {
			return err
		}
		fmt.Fprintf(w, "Scheduled modules to be reprocessed for appVersion > %q and status = %d.", appVersion, code)
		return nil
	}

	// Reprocess only versions with version type release and status of 200 or 290.
	releaseOnly := r.FormValue("release_only") == "true"
	if releaseOnly {
		if err := s.db.UpdateModuleVersionStatesForReprocessingReleaseVersionsOnly(r.Context(), appVersion); err != nil {
			return err
		}
		fmt.Fprintf(w, "Scheduled release and non-incompatible version of modules to be reprocessed for appVersion > %q.", appVersion)
		return nil
	}

	// Reprocess all module versions in module_version_states.
	if err := s.db.UpdateModuleVersionStatesForReprocessing(r.Context(), appVersion); err != nil {
		return err
	}
	fmt.Fprintf(w, "Scheduled modules to be reprocessed for appVersion > %q.", appVersion)
	return nil
}

func (s *Server) clearCache(w http.ResponseWriter, r *http.Request) error {
	if s.cache == nil {
		return errors.New("redis cache client is not configured")
	}
	if err := s.cache.Clear(r.Context()); err != nil {
		return err
	}
	fmt.Fprint(w, "Cache cleared.")
	return nil
}

// handleDelete deletes the specified module version.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) error {
	modulePath, version, err := parseModulePathAndVersion(r.URL.Path)
	if err != nil {
		return &serverError{http.StatusBadRequest, err}
	}
	if err := s.db.DeleteModule(r.Context(), modulePath, version); err != nil {
		return &serverError{http.StatusInternalServerError, err}
	}
	fmt.Fprintf(w, "Deleted %s@%s", modulePath, version)
	return nil
}

// Consider a module version for cleaning only if it is older than this.
const cleanDays = 7

// handleClean handles a request to clean module versions.
//
// If the request has a 'limit' query parameter, then up to that many module versions
// are selected from the DB among those eligible for cleaning, and they are cleaned.
//
// If the request has a 'module' query parameter, all versions of that module path
// are cleaned.
//
// It is an error if neither or both query parameters are provided.
func (s *Server) handleClean(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "handleClean")
	ctx := r.Context()

	limit := r.FormValue("limit")
	module := r.FormValue("module")
	switch {
	case limit == "" && module == "":
		return errors.New("need 'limit' or 'module' query param")

	case limit != "" && module != "":
		return errors.New("need exactly one of 'limit' or 'module' query param")

	case limit != "":
		mvs, err := s.db.GetModuleVersionsToClean(ctx, cleanDays, parseLimitParam(r, 1000))
		if err != nil {
			return err
		}
		log.Infof(ctx, "cleaning %d modules", len(mvs))
		if err := s.db.CleanModuleVersions(ctx, mvs, "Bulk deleted via /clean endpoint"); err != nil {
			return err
		}
		fmt.Fprintf(w, "Cleaned %d module versions.\n", len(mvs))
		return nil

	default: // module != ""
		log.Infof(ctx, "cleaning module %q", module)
		if err := s.db.CleanAllModuleVersions(ctx, module, "Manually deleted via /clean endpoint"); err != nil {
			return err
		}
		fmt.Fprintf(w, "Cleaned module %q\n", module)
		return nil
	}
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Underlying().Ping(); err != nil {
		http.Error(w, fmt.Sprintf("DB ping failed: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
}

// Parse the template for the status page.
func parseTemplate(staticPath, filename template.TrustedSource) (*template.Template, error) {
	if staticPath.String() == "" {
		return nil, nil
	}
	templatePath := template.TrustedSourceJoin(staticPath, template.TrustedSourceFromConstant("/worker"), filename)
	return template.New(filename.String()).Funcs(template.FuncMap{
		"truncate":  truncate,
		"timefmt":   formatTime,
		"bytesToMi": bytesToMi,
		"pct":       percentage,
		"timeSince": func(t time.Time) time.Duration {
			return time.Since(t).Round(time.Second)
		},
		"timeSub": func(t1, t2 time.Time) time.Duration {
			return t1.Sub(t2).Round(time.Second)
		},
	}).ParseFilesFromTrustedSources(templatePath)
}

func truncate(length int, text *string) *string {

	if text == nil {
		return nil
	}
	if len(*text) <= length {
		return text
	}
	s := (*text)[:length] + "..."
	return &s
}

var locNewYork *time.Location

func init() {
	var err error
	locNewYork, err = time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf(context.Background(), "time.LoadLocation: %v", err)
	}
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "Never"
	}
	return t.In(locNewYork).Format("2006-01-02 15:04:05")
}

// bytesToMi converts an integral value of bytes into mebibytes.
func bytesToMi(b uint64) uint64 {
	return b / (1024 * 1024)
}

// percentage computes the truncated percentage of x/y.
// It returns 0 if y is 0.
// x and y can be any int or uint type.
func percentage(x, y interface{}) int {
	denom := toUint64(y)
	if denom == 0 {
		return 0
	}
	return int(toUint64(x) * 100 / denom)
}

func toUint64(n interface{}) uint64 {
	v := reflect.ValueOf(n)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(v.Int())
	default: // assume uint
		return v.Uint()
	}
}

// parseLimitParam parses the query parameter "limit" as an integer. If the
// parameter is missing or there is a parse error, it is logged and the default
// value is returned.
func parseLimitParam(r *http.Request, defaultValue int) int {
	const name = "limit"
	param := r.FormValue(name)
	if param == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(param)
	if err != nil {
		log.Errorf(r.Context(), "parsing query parameter %q: %v", name, err)
		return defaultValue
	}
	return val
}

type serverError struct {
	status int   // HTTP status code
	err    error // wrapped error
}

func (s *serverError) Error() string {
	return fmt.Sprintf("%d (%s): %v", s.status, http.StatusText(s.status), s.err)
}

// errorHandler converts a function that returns an error into an http.HandlerFunc.
func (s *Server) errorHandler(f func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			s.serveError(w, r, err)
		}
	}
}

func (s *Server) serveError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	serr, ok := err.(*serverError)
	if !ok {
		serr = &serverError{status: http.StatusInternalServerError, err: err}
	}
	if serr.status == http.StatusInternalServerError {
		log.Error(ctx, serr.err)
		s.reportError(ctx, err, w, r)
	} else {
		log.Infof(ctx, "returning %d (%s) for error %v", serr.status, http.StatusText(serr.status), err)
	}
	http.Error(w, serr.err.Error(), serr.status)
}

// mib is the number of bytes in a mebibyte (Mi).
const mib = 1024 * 1024

// The largest module zip size we can comfortably process.
// We probably will OOM if we process a module whose zip is larger.
var maxModuleZipSize int64 = math.MaxInt64

func init() {
	v := config.GetEnvInt(context.Background(), "GO_DISCOVERY_MAX_MODULE_ZIP_MI", -1)
	if v > 0 {
		maxModuleZipSize = int64(v) * mib
	}
}

func (s *Server) setLoadShedder(ctx context.Context) {
	mebis := config.GetEnvInt(ctx, "GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI", -1)
	if mebis > 0 {
		log.Infof(ctx, "shedding load over %dMi", mebis)
		s.loadShedder = &loadShedder{
			maxSizeInFlight: uint64(mebis) * mib,
			getDBInfo:       s.workerDBInfo,
		}
	}
}

// ZipLoadShedStats returns a snapshot of the current LoadShedStats for zip files.
func (s *Server) ZipLoadShedStats() LoadShedStats {
	if s.loadShedder != nil {
		return s.loadShedder.stats()
	}
	return LoadShedStats{}
}
