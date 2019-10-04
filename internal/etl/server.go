// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/xerrors"
)

// Server can be installed to serve the go discovery etl.
type Server struct {
	indexClient *index.Client
	proxyClient *proxy.Client
	db          *postgres.DB
	queue       Queue

	indexTemplate *template.Template
}

// NewServer creates a new Server with the given dependencies.
func NewServer(db *postgres.DB,
	indexClient *index.Client,
	proxyClient *proxy.Client,
	queue Queue,
	staticPath string,
) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer(db, ic, pc, q, %q)", staticPath)

	indexTemplate, err := parseTemplate(staticPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		db:          db,
		indexClient: indexClient,
		proxyClient: proxyClient,
		queue:       queue,

		indexTemplate: indexTemplate,
	}, nil
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler)) {
	// cloud-scheduler: poll-and-queue polls the Module Index for new versions
	// that have been published and inserts that metadata into
	// module_version_states. It also inserts the version into the task-queue
	// to to be fetched and processed.
	// This endpoint is invoked by a Cloud Scheduler job:
	// 	// See the note about duplicate tasks for "/requeue" below.
	handle("/poll-and-queue", http.HandlerFunc(s.handleIndexAndQueue))

	// cloud-scheduler: refresh-search is used to refresh data in mvw_search_documents. This
	// is in the process of being deprecated in favor of using
	// search_documents for storing search data (b/136674524).
	// This endpoint is invoked by a Cloud Scheduler job:
	// 	handle("/refresh-search", http.HandlerFunc(s.handleRefreshSearch))

	// cloud-scheduler: update-imported-by-count updates the imported_by_count for packages
	// in search_documents where imported_by_count_updated_at is null or
	// imported_by_count_updated_at < version_updated_at.
	// This endpoint is invoked by a Cloud Scheduler job:
	// 	handle("/update-imported-by-count", http.HandlerFunc(s.handleUpdateImportedByCount))

	// task-queue: fetch fetches a module version from the Module Mirror, and
	// processes the contents, and inserts it into the database. If a fetch
	// request fails for any reason other than an http.StatusInternalServerError,
	// it will return an http.StatusOK so that the task queue does not retry
	// fetching module versions that have a terminal error.
	// This endpoint is invoked by a Cloud Tasks queue:
	// 	handle("/fetch/", http.StripPrefix("/fetch", http.HandlerFunc(s.handleFetch)))

	// manual: requeue queries the module_version_states table for the next
	// batch of module versions to process, and enqueues them for processing.
	// Normally this will not cause duplicate processing, because Cloud Tasks
	// are de-duplicated. That does not apply after a task has been finished or
	// deleted for one hour (see
	// https://cloud.google.com/tasks/docs/reference/rpc/google.cloud.tasks.v2#createtaskrequest,
	// under "Task De-duplication"). If you cannot wait an hour, you can force
	// duplicate tasks by providing any string as the "suffix" query parameter.
	handle("/requeue", http.HandlerFunc(s.handleRequeue))

	// manual: reprocess sets status = 505 for all records in the
	// module_version_states table that were processed by an app_version
	// that occurred after the provided app_version param, so that they
	// will be scheduled for reprocessing the next time a request to
	// /requeue is made.
	handle("/reprocess", http.HandlerFunc(s.handleReprocess))

	// manual: populate-stdlib inserts all versions of the Go standard
	// library into the tasks queue to be processed and inserted into the
	 handlePopulateStdLib should be updated whenever a new
	// version of Go is released.
	// see the comments on duplicate tasks for "/requeue", above.
	handle("/populate-stdlib", http.HandlerFunc(s.handlePopulateStdLib))

	// manual: populate-search-documents inserts a record into
	// search_documents for all paths in the packages table that do not
	// exist in search_documents.
	handle("/populate-search-documents", http.HandlerFunc(s.handlePopulateSearchDocuments))

	// returns the ETL homepage.
	handle("/", http.HandlerFunc(s.handleStatusPage))
}

// handleUpdateImportedByCount updates imported_by_count for packages in
// search_documents where imported_by_count_updated_at < version_updated_at or
// imported_by_count_updated_at is null.
func (s *Server) handleUpdateImportedByCount(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 1000)
	if err := s.db.UpdateSearchDocumentsImportedByCount(r.Context(), limit); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Errorf("db.UpdateSearchDocumentsImportedByCount(ctx, %d): %v", limit, err)
	}
}

// handlePopulateSearchDocuments inserts a record into search_documents for all
// package_paths that exist in packages but not in search_documents.
func (s *Server) handlePopulateSearchDocuments(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 100)
	ctx := r.Context()
	log.Infof("Populating search documents for %d packages", limit)
	pkgPaths, err := s.db.GetPackagesForSearchDocumentUpsert(ctx, limit)
	if err != nil {
		log.Errorf("s.db.GetPackagesSearchDocumentUpsert(ctx): %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	for _, path := range pkgPaths {
		if err := s.db.UpsertSearchDocument(ctx, path); err != nil {
			log.Errorf("s.db.UpsertSearchDocument(ctx, %q): %v", path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
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
	if msg == "" {
		msg = "[empty message]"
	}
	log.Info(msg)

	if code == http.StatusInternalServerError {
		log.Infof("doFetch of %s returned %d; returning that code to retry task", r.URL.Path, code)
		http.Error(w, http.StatusText(code), code)
		return
	}
	if code/100 == 2 {
		log.Infof("doFetch of %s succeeded with %d", r.URL.Path, code)
	} else {
		log.Infof("doFetch of %s returned code %d; returning OK to avoid retry", r.URL.Path, code)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if code/100 == 2 {
		fmt.Fprintln(w, msg)
	}
	fmt.Fprintln(w, http.StatusText(code))
}

// doFetch executes a fetch request and returns the msg and status.
func (s *Server) doFetch(r *http.Request) (string, int) {
	isb, err := s.db.IsExcluded(r.Context(), r.URL.Path)
	if err != nil {
		return err.Error(), http.StatusInternalServerError
	}
	if isb {
		return "blacklisted", http.StatusForbidden
	}
	modulePath, version, err := parseModulePathAndVersion(r.URL.Path)
	if err != nil {
		return err.Error(), http.StatusBadRequest
	}

	code, err := fetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.db)
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

func (s *Server) handleRefreshSearch(w http.ResponseWriter, r *http.Request) {
	if err := s.db.RefreshSearchDocuments(r.Context()); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Error(err)
		return
	}
}

func (s *Server) handleIndexAndQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := parseIntParam(r, "limit", 10)
	suffixParam := r.FormValue("suffix")
	since, err := s.db.LatestIndexTimestamp(ctx)
	if err != nil {
		log.Errorf("doing proxy index update: %v", err)
		http.Error(w, "error doing proxy index update", http.StatusInternalServerError)
		return
	}
	versions, err := s.indexClient.GetVersions(ctx, since, limit)
	if err != nil {
		log.Errorf("getting index versions: %v", err)
		http.Error(w, "error getting versions", http.StatusInternalServerError)
		return
	}
	if err := s.db.InsertIndexVersions(ctx, versions); err != nil {
		log.Error(err)
		http.Error(w, "error inserting versions", http.StatusInternalServerError)
		return
	}
	for _, version := range versions {
		if err := s.queue.ScheduleFetch(ctx, version.Path, version.Version, suffixParam); err != nil {
			log.Errorf("scheduling fetch: %v", err)
			http.Error(w, "error scheduling fetch", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		fmt.Fprintf(w, "scheduled %s@%s\n", v.Path, v.Version)
	}
}

// handleRequeue queries the module_version_states table for the next
// batch of module versions to process, and enqueues them for processing.  Note
// that this may cause duplicate processing.
func (s *Server) handleRequeue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := parseIntParam(r, "limit", 10)
	suffixParam := r.FormValue("suffix") // append to task name to avoid deduplication
	span := trace.FromContext(r.Context())
	span.Annotate([]trace.Attribute{trace.Int64Attribute("limit", int64(limit))}, "processed limit")
	versions, err := s.db.GetNextVersionsToFetch(ctx, limit)
	if err != nil {
		log.Error(err)
		http.Error(w, "error getting versions to fetch", http.StatusInternalServerError)
		return
	}
	log.Infof("Got %d versions to fetch", len(versions))

	span.Annotate([]trace.Attribute{trace.Int64Attribute("versions to fetch", int64(len(versions)))}, "processed limit")
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		if err := s.queue.ScheduleFetch(ctx, v.ModulePath, v.Version, suffixParam); err != nil {
			log.Errorf("scheduling fetch: %v", err)
			http.Error(w, "error scheduling fetch", http.StatusInternalServerError)
			return
		}
	}
}

// handleStatusPage serves the etl status page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	msg, err := s.doStatusPage(w, r)
	if err != nil {
		log.Error(err)
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
	page := struct {
		Stats                        *postgres.VersionStats
		Next, Recent, RecentFailures []*internal.VersionState
	}{
		Stats:          stats,
		Next:           next,
		Recent:         recents,
		RecentFailures: failures,
	}
	var buf bytes.Buffer
	if err := s.indexTemplate.Execute(&buf, page); err != nil {
		return "error rendering template", err
	}
	if _, err := io.Copy(w, &buf); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Errorf("Error copying buffer to ResponseWriter: %v", err)
	}
	return "", nil
}

func (s *Server) handlePopulateStdLib(w http.ResponseWriter, r *http.Request) {
	msg, err := s.doPopulateStdLib(r.Context(), r.FormValue("suffix"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil {
		log.Errorf("handlePopulateStdLib: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		log.Infof("handlePopulateStdLib: %s", msg)
		_, _ = io.WriteString(w, msg)
	}
}

func (s *Server) doPopulateStdLib(ctx context.Context, suffix string) (string, error) {
	versions, err := stdlib.Versions()
	if err != nil {
		return "", err
	}
	for _, v := range versions {
		if err := s.queue.ScheduleFetch(ctx, stdlib.ModulePath, v, suffix); err != nil {
			return "", xerrors.Errorf("Error scheduling fetch for %s: %w", v, err)
		}
	}
	return fmt.Sprintf("Scheduled fetches for %s.\n", strings.Join(versions, ", ")), nil
}

func (s *Server) handleReprocess(w http.ResponseWriter, r *http.Request) {
	appVersion := r.FormValue("app_version")
	if appVersion == "" {
		log.Error("app_version was not specified")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := config.ValidateAppVersion(appVersion); err != nil {
		log.Errorf("config.ValidateAppVersion(%q): %v", appVersion, err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateVersionStatesForReprocessing(r.Context(), appVersion); err != nil {
		log.Error(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// Parse the template for the status page.
func parseTemplate(staticPath string) (*template.Template, error) {
	if staticPath == "" {
		return nil, nil
	}
	templatePath := filepath.Join(staticPath, "html/etl/index.tmpl")
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
		log.Fatalf("time.LoadLocation: %v", err)
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
		log.Errorf("parsing query parameter %q: %v", name, err)
		return defaultValue
	}
	return val
}
