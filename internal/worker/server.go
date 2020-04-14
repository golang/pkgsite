// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/errorreporting"
	"github.com/go-redis/redis/v7"
	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/queue"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
)

// Server can be installed to serve the go discovery worker.
type Server struct {
	cfg             *config.Config
	indexClient     *index.Client
	proxyClient     *proxy.Client
	sourceClient    *source.Client
	redisClient     *redis.Client
	db              *postgres.DB
	queue           queue.Queue
	reportingClient *errorreporting.Client

	indexTemplate *template.Template
}

// NewServer creates a new Server with the given dependencies.
func NewServer(cfg *config.Config,
	db *postgres.DB,
	indexClient *index.Client,
	proxyClient *proxy.Client,
	sourceClient *source.Client,
	redisClient *redis.Client,
	queue queue.Queue,
	reportingClient *errorreporting.Client,
	staticPath string,
) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(db, ic, pc, q, %q)", staticPath)

	indexTemplate, err := parseTemplate(staticPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:             cfg,
		db:              db,
		indexClient:     indexClient,
		proxyClient:     proxyClient,
		sourceClient:    sourceClient,
		redisClient:     redisClient,
		queue:           queue,
		reportingClient: reportingClient,
		indexTemplate:   indexTemplate,
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
	// cloud-scheduler: poll-and-queue polls the Module Index for new versions
	// that have been published and inserts that metadata into
	// module_version_states. It also inserts the version into the task-queue
	// to to be fetched and processed.
	// This endpoint is invoked by a Cloud Scheduler job.
	// See the note about duplicate tasks for "/requeue" below.
	handle("/poll-and-queue", rmw(s.errorHandler(s.handleIndexAndQueue)))

	// cloud-scheduler: update-imported-by-count updates the imported_by_count for packages
	// in search_documents where imported_by_count_updated_at is null or
	// imported_by_count_updated_at < version_updated_at.
	// This endpoint is invoked by a Cloud Scheduler job.
	handle("/update-imported-by-count", rmw(s.errorHandler(s.handleUpdateImportedByCount)))

	// cloud-scheduler: download search document data and update the redis sorted
	// set(s) used in auto-completion.
	handle("/update-redis-indexes", rmw(s.errorHandler(s.handleUpdateRedisIndexes)))

	// task-queue: fetch fetches a module version from the Module Mirror, and
	// processes the contents, and inserts it into the database. If a fetch
	// request fails for any reason other than an http.StatusInternalServerError,
	// it will return an http.StatusOK so that the task queue does not retry
	// fetching module versions that have a terminal error.
	// This endpoint is invoked by a Cloud Tasks queue.
	handle("/fetch/", http.StripPrefix("/fetch", http.HandlerFunc(s.handleFetch)))

	// manual: requeue queries the module_version_states table for the next
	// batch of module versions to process, and enqueues them for processing.
	// Normally this will not cause duplicate processing, because Cloud Tasks
	// are de-duplicated. That does not apply after a task has been finished or
	// deleted for one hour (see
	// https://cloud.google.com/tasks/docs/reference/rpc/google.cloud.tasks.v2#createtaskrequest,
	// under "Task De-duplication"). If you cannot wait an hour, you can force
	// duplicate tasks by providing any string as the "suffix" query parameter.
	handle("/requeue", rmw(s.errorHandler(s.handleRequeue)))

	// manual: reprocess sets status = 505 for all records in the
	// module_version_states table that were processed by an app_version
	// that occurred after the provided app_version param, so that they
	// will be scheduled for reprocessing the next time a request to
	// /requeue is made.
	handle("/reprocess", rmw(s.errorHandler(s.handleReprocess)))

	// manual: populate-stdlib inserts all versions of the Go standard
	// library into the tasks queue to be processed and inserted into the
	// database. handlePopulateStdLib should be updated whenever a new
	// version of Go is released.
	// see the comments on duplicate tasks for "/requeue", above.
	handle("/populate-stdlib", rmw(s.errorHandler(s.handlePopulateStdLib)))

	// manual: populate-search-documents inserts a record into
	// search_documents for all paths in the packages table that do not
	// exist in search_documents.
	handle("/repopulate-search-documents", rmw(s.errorHandler(s.handleRepopulateSearchDocuments)))

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
	limit := parseIntParam(r, "limit", 100)
	beforeParam := r.FormValue("before")
	if beforeParam == "" {
		return &serverError{
			http.StatusBadRequest,
			errors.New("must provide 'before' query param as an RFC3339 datetime"),
		}
	}
	before, err := time.Parse(beforeParam, time.RFC3339)
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
		if err := s.db.UpsertSearchDocument(ctx, args); err != nil {
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

	code, err := FetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.sourceClient, s.db)
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

func (s *Server) handleIndexAndQueue(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit := parseIntParam(r, "limit", 10)
	suffixParam := r.FormValue("suffix")
	since, err := s.db.LatestIndexTimestamp(ctx)
	if err != nil {
		return err
	}
	versions, err := s.indexClient.GetVersions(ctx, since, limit)
	if err != nil {
		return err
	}
	if err := s.db.InsertIndexVersions(ctx, versions); err != nil {
		return err
	}
	for _, version := range versions {
		if err := s.queue.ScheduleFetch(ctx, version.Path, version.Version, suffixParam); err != nil {
			return fmt.Errorf("scheduling fetch: %v", err)
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		fmt.Fprintf(w, "scheduled %s@%s\n", v.Path, v.Version)
	}
	return nil
}

// handleRequeue queries the module_version_states table for the next
// batch of module versions to process, and enqueues them for processing.  Note
// that this may cause duplicate processing.
func (s *Server) handleRequeue(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit := parseIntParam(r, "limit", 10)
	suffixParam := r.FormValue("suffix") // append to task name to avoid deduplication
	span := trace.FromContext(r.Context())
	span.Annotate([]trace.Attribute{trace.Int64Attribute("limit", int64(limit))}, "processed limit")
	versions, err := s.db.GetNextVersionsToFetch(ctx, limit)
	if err != nil {
		return err
	}
	log.Infof(ctx, "Got %d versions to fetch", len(versions))

	span.Annotate([]trace.Attribute{trace.Int64Attribute("versions to fetch", int64(len(versions)))}, "processed limit")
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		if err := s.queue.ScheduleFetch(ctx, v.ModulePath, v.Version, suffixParam); err != nil {
			return fmt.Errorf("scheduling fetch: %v", err)
		}
	}
	return nil
}

// handleStatusPage serves the worker status page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	msg, err := s.doStatusPage(w, r)
	if err != nil {
		log.Error(r.Context(), err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// doStatusPage writes the status page. On error it returns the error and a short
// string to be written back to the client.
func (s *Server) doStatusPage(w http.ResponseWriter, r *http.Request) (string, error) {
	ctx := r.Context()
	const pageSize = 20
	next, err := s.db.GetNextVersionsToFetch(ctx, pageSize)
	if err != nil {
		return "error fetching next versions", err
	}
	failures, err := s.db.GetRecentFailedVersions(ctx, pageSize)
	if err != nil {
		return "error fetching recent failures", err
	}
	recents, err := s.db.GetRecentVersions(ctx, pageSize)
	if err != nil {
		return "error fetching recent versions", err
	}
	stats, err := s.db.GetVersionStats(ctx)
	if err != nil {
		return "error fetching stats", err
	}

	type count struct {
		Code  int
		Desc  string
		Count int
	}
	var counts []*count
	for code, n := range stats.VersionCounts {
		c := &count{Code: code, Count: n}
		if e := derrors.FromHTTPStatus(code, ""); e != nil && e != derrors.Unknown {
			c.Desc = e.Error()
		} else {
			switch code {
			case hasIncompletePackagesCode:
				c.Desc = hasIncompletePackagesDesc
			case 505:
				c.Desc = "needs reprocessing"
			default:
				c.Desc = http.StatusText(code)
			}
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
	}{
		Config:          s.cfg,
		Env:             env,
		ResourcePrefix:  strings.ToLower(env) + "-",
		LatestTimestamp: &stats.LatestTimestamp,
		Counts:          counts,
		Next:            next,
		Recent:          recents,
		RecentFailures:  failures,
	}
	var buf bytes.Buffer
	if err := s.indexTemplate.Execute(&buf, page); err != nil {
		return "error rendering template", err
	}
	if _, err := io.Copy(w, &buf); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Errorf(ctx, "Error copying buffer to ResponseWriter: %v", err)
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
		if err := s.queue.ScheduleFetch(ctx, stdlib.ModulePath, v, suffix); err != nil {
			return "", fmt.Errorf("error scheduling fetch for %s: %w", v, err)
		}
	}
	return fmt.Sprintf("Scheduled fetches for %s.\n", strings.Join(versions, ", ")), nil
}

func (s *Server) handleReprocess(w http.ResponseWriter, r *http.Request) error {
	appVersion := r.FormValue("app_version")
	if appVersion == "" {
		return &serverError{http.StatusBadRequest, errors.New("app_version was not specified")}
	}
	if err := config.ValidateAppVersion(appVersion); err != nil {
		return &serverError{http.StatusBadRequest, fmt.Errorf("config.ValidateAppVersion(%q): %v", appVersion, err)}
	}
	if err := s.db.UpdateModuleVersionStatesForReprocessing(r.Context(), appVersion); err != nil {
		return err
	}
	return nil
}

// Parse the template for the status page.
func parseTemplate(staticPath string) (*template.Template, error) {
	if staticPath == "" {
		return nil, nil
	}
	templatePath := filepath.Join(staticPath, "html/worker/index.tmpl")
	return template.New("index.tmpl").Funcs(template.FuncMap{
		"truncate": truncate,
		"timefmt":  formatTime,
	}).ParseFiles(templatePath)
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

// parseIntParam parses the query parameter with name as in integer. If the
// parameter is missing or there is a parse error, it is logged and the default
// value is returned.
func parseIntParam(r *http.Request, name string, defaultValue int) int {
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
