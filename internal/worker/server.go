// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/errorreporting"
	"github.com/go-redis/redis/v7"
	"github.com/google/safehtml/template"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/sync/errgroup"
)

// Server can be installed to serve the go discovery worker.
type Server struct {
	cfg                  *config.Config
	indexClient          *index.Client
	proxyClient          *proxy.Client
	sourceClient         *source.Client
	redisHAClient        *redis.Client
	redisCacheClient     *redis.Client
	db                   *postgres.DB
	queue                queue.Queue
	reportingClient      *errorreporting.Client
	taskIDChangeInterval time.Duration

	indexTemplate *template.Template
}

// ServerConfig contains everything needed by a Server.
type ServerConfig struct {
	DB                   *postgres.DB
	IndexClient          *index.Client
	ProxyClient          *proxy.Client
	SourceClient         *source.Client
	RedisHAClient        *redis.Client
	RedisCacheClient     *redis.Client
	Queue                queue.Queue
	ReportingClient      *errorreporting.Client
	TaskIDChangeInterval time.Duration
	StaticPath           template.TrustedSource
}

// NewServer creates a new Server with the given dependencies.
func NewServer(cfg *config.Config, scfg ServerConfig) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(db, %+v)", scfg)

	indexTemplate, err := parseTemplate(scfg.StaticPath, template.TrustedSourceFromConstant("index.tmpl"))
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:                  cfg,
		db:                   scfg.DB,
		indexClient:          scfg.IndexClient,
		proxyClient:          scfg.ProxyClient,
		sourceClient:         scfg.SourceClient,
		redisHAClient:        scfg.RedisHAClient,
		redisCacheClient:     scfg.RedisCacheClient,
		queue:                scfg.Queue,
		reportingClient:      scfg.ReportingClient,
		taskIDChangeInterval: scfg.TaskIDChangeInterval,
		indexTemplate:        indexTemplate,
	}, nil
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler)) {
	// rmw wires in error reporting to the handler. It is configured here, in
	// Install, because not every handler should have error reporting. For
	// example, we don't want to get an error report each time a /fetch fails.
	rmw := middleware.Identity()
	if s.reportingClient != nil {
		rmw = middleware.ErrorReporting(s.reportingClient.Report)
	}

	// scheduled: poll polls the Module Index for new modules
	// that have been published and inserts that metadata into
	// module_version_states.
	// This endpoint is intended to be invoked periodically by a scheduler.
	// See the note about duplicate tasks for "/enqueue" below.
	handle("/poll", rmw(s.errorHandler(s.handlePollIndex)))

	// TODO: remove after /poll is in production and the scheduler jobs have been changed.
	// scheduled: poll-and-queue polls the Module Index for new modules
	// that have been published and inserts that metadata into
	// module_version_states. It also inserts the version into the task-queue
	// to to be fetched and processed.
	// This endpoint is intended to be invoked periodically by a scheduler.
	// See the note about duplicate tasks for "/requeue" below.
	handle("/poll-and-queue", rmw(s.errorHandler(s.handleIndexAndQueue)))

	// scheduled: update-imported-by-count update the imported_by_count for
	// packages in search_documents where imported_by_count_updated_at is null
	// or imported_by_count_updated_at < version_updated_at.
	// This endpoint is intended to be invoked periodically by a scheduler.
	handle("/update-imported-by-count", rmw(s.errorHandler(s.handleUpdateImportedByCount)))

	// scheduled: download search document data and update the redis sorted
	// set(s) used in auto-completion.
	handle("/update-redis-indexes", rmw(s.errorHandler(s.handleUpdateRedisIndexes)))

	// task-queue: fetch fetches a module version from the Module Mirror, and
	// processes the contents, and inserts it into the database. If a fetch
	// request fails for any reason other than an http.StatusInternalServerError,
	// it will return an http.StatusOK so that the task queue does not retry
	// fetching module versions that have a terminal error.
	// This endpoint is intended to be invoked by a task queue with semantics like
	// Google Cloud Task Queues.
	handle("/fetch/", http.StripPrefix("/fetch", http.HandlerFunc(s.handleFetch)))

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
	// deleted for Server.taskIDChangeInterval (see
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

	// returns the Worker homepage.
	handle("/", http.HandlerFunc(s.handleStatusPage))
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
	if r.URL.Path == "/favicon.ico" {
		return
	}

	msg, code := s.doFetch(r)
	if code == http.StatusInternalServerError {
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
func (s *Server) doFetch(r *http.Request) (string, int) {
	modulePath, version, err := parseModulePathAndVersion(r.URL.Path)
	if err != nil {
		return err.Error(), http.StatusBadRequest
	}

	code, err := FetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.sourceClient, s.db, s.cfg.AppVersionLabel())
	if err != nil {
		return err.Error(), code
	}
	return fmt.Sprintf("fetched and updated %s@%s", modulePath, version), code
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
		return modulePath, internal.LatestVersion, nil
	}
	parts := strings.Split(p, "/@v/")
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
	return nil
}

func (s *Server) handleIndexAndQueue(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "handleIndexAndQueue(%q)", r.URL.Path)
	ctx := r.Context()
	limit := parseLimitParam(r, 10)
	suffixParam := r.FormValue("suffix")
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
	log.Infof(ctx, "Scheduling modules to be fetched: %d new modules from index.golang.org", len(modules))
	for _, version := range modules {
		if err := s.queue.ScheduleFetch(ctx, version.Path, version.Version, suffixParam, s.taskIDChangeInterval); err != nil {
			return err
		}
	}
	log.Infof(ctx, "Successfully scheduled modules to be fetched: %d new modules from index.golang.org", len(modules))

	w.Header().Set("Content-Type", "text/plain")
	for _, v := range modules {
		fmt.Fprintf(w, "scheduled %s@%s\n", v.Path, v.Version)
	}
	return nil
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
	for _, m := range modules {
		if err := s.queue.ScheduleFetch(ctx, m.ModulePath, m.Version, suffixParam, s.taskIDChangeInterval); err != nil {
			return err
		}
	}
	log.Infof(ctx, "Successfully scheduled modules to be fetched: %d modules queued", len(modules))

	return nil
}

// handleStatusPage serves the worker status page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	msg, err := s.doStatusPage(w, r)
	if err != nil {
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// doStatusPage writes the status page. On error it returns the error and a short
// string to be written back to the client.
func (s *Server) doStatusPage(w http.ResponseWriter, r *http.Request) (_ string, err error) {
	defer derrors.Wrap(&err, "doStatusPage")
	const pageSize = 20
	var (
		next, failures, recents []*internal.ModuleVersionState
		stats                   *postgres.VersionStats
		experiments             []*internal.Experiment
		excluded                []string
	)
	type annotation struct {
		error
		msg string
	}
	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() error {
		var err error
		next, err = s.db.GetNextModulesToFetch(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching next versions"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		failures, err = s.db.GetRecentFailedVersions(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching recent failures"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		recents, err = s.db.GetRecentVersions(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching recent versions"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		stats, err = s.db.GetVersionStats(ctx)
		if err != nil {
			return annotation{err, "error fetching stats"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		experiments, err = s.db.GetExperiments(ctx)
		if err != nil {
			return annotation{err, "error fetching experiments"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		excluded, err = s.db.GetExcludedPrefixes(ctx)
		if err != nil {
			return annotation{err, "error fetching excluded"}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		var e annotation
		if errors.As(err, &e) {
			return e.msg, err
		}
		return "", err
	}

	type count struct {
		Code  int
		Desc  string
		Count int
	}
	var counts []*count
	for code, n := range stats.VersionCounts {
		c := &count{Code: code, Count: n}
		if e := derrors.FromStatus(code, ""); e != nil && e != derrors.Unknown {
			c.Desc = e.Error()
		}
		counts = append(counts, c)
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].Code < counts[j].Code })

	var env string
	switch s.cfg.ServiceID {
	case "":
		env = "Local"
	case "dev-etl":
		env = "Dev"
	case "staging-etl":
		env = "Staging"
	case "etl":
		env = "Prod"
	}
	page := struct {
		Config                       *config.Config
		Env                          string
		ResourcePrefix               string
		LatestTimestamp              *time.Time
		Counts                       []*count
		Next, Recent, RecentFailures []*internal.ModuleVersionState
		Experiments                  []*internal.Experiment
		Excluded                     []string
	}{
		Config:          s.cfg,
		Env:             env,
		ResourcePrefix:  strings.ToLower(env) + "-",
		LatestTimestamp: &stats.LatestTimestamp,
		Counts:          counts,
		Next:            next,
		Recent:          recents,
		RecentFailures:  failures,
		Experiments:     experiments,
		Excluded:        excluded,
	}
	var buf bytes.Buffer
	if err := s.indexTemplate.Execute(&buf, page); err != nil {
		return "error rendering template", err
	}
	if _, err := io.Copy(w, &buf); err != nil {
		log.Errorf(ctx, "Error copying buffer to ResponseWriter: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
	return "", nil
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
		if err := s.queue.ScheduleFetch(ctx, stdlib.ModulePath, v, suffix, s.taskIDChangeInterval); err != nil {
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

	if err := s.db.UpdateModuleVersionStatesForReprocessing(r.Context(), appVersion); err != nil {
		return err
	}
	fmt.Fprintf(w, "Scheduled modules to be reprocessed for appVersion > %q.", appVersion)
	return nil
}

func (s *Server) clearCache(w http.ResponseWriter, r *http.Request) error {
	if s.redisCacheClient == nil {
		return errors.New("redis cache client is not configured")
	}
	status := s.redisCacheClient.FlushAll()
	if status.Err() != nil {
		return status.Err()
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

// Parse the template for the status page.
func parseTemplate(staticPath, filename template.TrustedSource) (*template.Template, error) {
	if staticPath.String() == "" {
		return nil, nil
	}
	templatePath := template.TrustedSourceJoin(staticPath, template.TrustedSourceFromConstant("html/worker"), filename)
	return template.New(filename.String()).Funcs(template.FuncMap{
		"truncate": truncate,
		"timefmt":  formatTime,
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

// parseLimitParam parses the query parameter with name as in integer. If the
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
	} else {
		log.Infof(ctx, "returning %d (%s) for error %v", serr.status, http.StatusText(serr.status), err)
	}
	http.Error(w, serr.err.Error(), serr.status)
}
