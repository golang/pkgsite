// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
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
	indexTemplate *template.Template,
) *Server {
	return &Server{
		db:          db,
		indexClient: indexClient,
		proxyClient: proxyClient,
		queue:       queue,

		indexTemplate: indexTemplate,
	}
}

// Install registers server routes using the given handler registration func.
func (s *Server) Install(handle func(string, http.Handler)) {
	// cloud-scheduler: poll-and-queue polls the Module Index for new versions
	// that have been published and inserts that metadata into
	// module_version_states. It also inserts the version into the task-queue
	// to to be fetched and processed.
	// This endpoint is invoked by a Cloud Scheduler job:
	// 	handle("/poll-and-queue", http.HandlerFunc(s.handleIndexAndQueue))

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
	// Note that this may cause duplicate processing.
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
	limitParam := r.FormValue("limit")
	var (
		limit = 1000
		err   error
	)
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("Error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	if err := s.db.UpdateSearchDocumentsImportedByCount(r.Context(), limit); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("db.UpdateSearchDocumentsImportedByCount(ctx, %d): %v", limit, err)
	}
}

// handlePopulateSearchDocuments inserts a record into search_documents for all
// package_paths that exist in packages but not in search_documents.
func (s *Server) handlePopulateSearchDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pkgPaths, err := s.db.GetPackagesForSearchDocumentUpsert(ctx)
	if err != nil {
		log.Printf("s.db.GetPackagesSearchDocumentUpsert(ctx): %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	for _, path := range pkgPaths {
		if err := s.db.UpsertSearchDocument(ctx, path); err != nil {
			log.Printf("s.db.UpsertSearchDocument(ctx, %q): %v", path, err)
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
	log.Println(msg)

	if code == http.StatusInternalServerError {
		http.Error(w, http.StatusText(code), code)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if code == http.StatusOK {
		fmt.Fprint(w, msg)
	}
	fmt.Fprint(w, http.StatusText(code))
}

// doFetch executes a fetch request and returns the msg and status.
func (s *Server) doFetch(r *http.Request) (string, int) {
	modulePath, version, err := parseModulePathAndVersion(r.URL.Path)
	if err != nil {
		return err.Error(), http.StatusBadRequest
	}

	code, err := fetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.db)
	if err != nil {
		return err.Error(), code
	}
	return fmt.Sprintf("Downloaded %s@%s\n", modulePath, version), http.StatusOK
}

// parseModulePathAndVersion returns the module and version specified by p. p
// is assumed to have the structure /<module>/@v/<version>.
func parseModulePathAndVersion(p string) (string, string, error) {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/@v/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path: %q", p)
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid path: %q", p)
	}
	return parts[0], parts[1], nil
}

func (s *Server) handleRefreshSearch(w http.ResponseWriter, r *http.Request) {
	if err := s.db.RefreshSearchDocuments(r.Context()); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) handleIndexAndQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limitParam := r.FormValue("limit")
	var (
		limit = 10
		err   error
	)
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("Error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	since, err := s.db.LatestIndexTimestamp(ctx)
	if err != nil {
		log.Printf("Error doing proxy index update: %v", err)
		http.Error(w, "error doing proxy index update", http.StatusInternalServerError)
		return
	}
	versions, err := s.indexClient.GetVersions(ctx, since, limit)
	if err != nil {
		log.Printf("Error getting index versions: %v", err)
		http.Error(w, "error getting versions", http.StatusInternalServerError)
		return
	}
	if err := s.db.InsertIndexVersions(ctx, versions); err != nil {
		log.Print(err)
		http.Error(w, "error inserting versions", http.StatusInternalServerError)
		return
	}
	for _, version := range versions {
		if err := s.queue.ScheduleFetch(ctx, version.Path, version.Version); err != nil {
			log.Printf("Error scheduling fetch: %v", err)
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
	limitParam := r.FormValue("limit")
	var (
		limit = 10
		err   error
	)
	span := trace.FromContext(r.Context())
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("Error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	span.Annotate([]trace.Attribute{trace.Int64Attribute("limit", int64(limit))}, "processed limit")
	versions, err := s.db.GetNextVersionsToFetch(ctx, limit)
	if err != nil {
		log.Print(err)
		http.Error(w, "error getting versions to fetch", http.StatusInternalServerError)
		return
	}
	span.Annotate([]trace.Attribute{trace.Int64Attribute("versions to fetch", int64(len(versions)))}, "processed limit")
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		if err := s.queue.ScheduleFetch(ctx, v.ModulePath, v.Version); err != nil {
			log.Printf("Error scheduling fetch: %v", err)
			http.Error(w, "error scheduling fetch", http.StatusInternalServerError)
			return
		}
	}
}

// handleStatusPage serves the etl status page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	msg, err := s.doStatusPage(w, r)
	if err != nil {
		log.Println(err)
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
		log.Printf("Error copying buffer to ResponseWriter: %v", err)
	}
	return "", nil
}

func (s *Server) handlePopulateStdLib(w http.ResponseWriter, r *http.Request) {
	// TODO(b/140413605): compute these versions from the Go repo, in internal/stdlib.

	// stdlibVersions is a map of each minor version of Go and the latest
	// patch version available for that minor version, according to
	// https://golang.org/doc/devel/release.html. This map will need to be
	// updated each time a new Go version is released.
	stdlibVersions := map[string]int{
		"v1.12": 7,
		"v1.11": 11,
	}
	// stdlibBetaVersions is a slice of beta versions available for Go.
	// This slice will need to be updated each time a new Go beta version
	// is released.
	stdlibBetaVersions := []string{"v1.13.0-beta.1"}

	var versionsToQueue [][]string
	for majMin, maxPatch := range stdlibVersions {
		for patch := 0; patch <= maxPatch; patch++ {
			v := fmt.Sprintf("%s.%d", majMin, patch)
			versionsToQueue = append(versionsToQueue, []string{"std", v})
		}
	}
	for _, betaVersion := range stdlibBetaVersions {
		versionsToQueue = append(versionsToQueue,
			[]string{"std", betaVersion})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, moduleVersion := range versionsToQueue {
		if err := s.queue.ScheduleFetch(r.Context(), moduleVersion[0], moduleVersion[1]); err != nil {
			log.Printf("Error scheduling fetch: %v", err)
			http.Error(w, "error scheduling fetch", http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) handleReprocess(w http.ResponseWriter, r *http.Request) {
	appVersion := r.FormValue("app_version")
	if appVersion == "" {
		log.Printf("app_version was not specified")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := config.ValidateAppVersion(appVersion); err != nil {
		log.Printf("config.ValidateAppVersion(%q): %v", appVersion, err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateVersionStatesForReprocessing(r.Context(), appVersion); err != nil {
		log.Print(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
