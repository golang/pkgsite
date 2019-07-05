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
	"golang.org/x/discovery/internal/dcensus"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"google.golang.org/appengine"
)

// Server is an http.Handler that implements functionality for managing the
// processing of new module versions.
type Server struct {
	http.Handler

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
	s := &Server{
		db:          db,
		indexClient: indexClient,
		proxyClient: proxyClient,
		queue:       queue,

		indexTemplate: indexTemplate,
	}
	r := dcensus.NewRouter()
	r.HandleFunc("/poll-and-queue/", s.handleIndexAndQueue)
	r.HandleFunc("/requeue/", s.handleRequeue)
	r.HandleFunc("/refresh-search/", s.handleRefreshSearch)
	r.HandleFunc("/populate-stdlib/", s.handlePopulateStdLib)
	r.Handle("/fetch/", http.StripPrefix("/fetch", http.HandlerFunc(s.handleFetch)))
	r.Handle("/queue-fetch/", http.StripPrefix("/queue-fetch", http.HandlerFunc(s.handleQueueFetch)))
	r.HandleFunc("/", s.handleStatusPage)
	s.Handler = r
	return s
}

// handleFetch executes a fetch requests and returns the outcome of that
// request.
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

	if code != http.StatusOK {
		http.Error(w, http.StatusText(code), code)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, msg)
}

// handleQueueFetch executes a fetch request and returns a http.StatusOK if the
// status is not http.StatusInternalServerError, so that the task queue does
// not retry fetching module versions that have a terminal error.
func (s *Server) handleQueueFetch(w http.ResponseWriter, r *http.Request) {
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
		return fmt.Sprintf("parseModulePathAndVersion(%q): %v", r.URL.Path, err), http.StatusBadRequest
	}

	code, err := fetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.db)
	if err != nil {
		return fmt.Sprintf("fetchAndUpdateState(r.Context(), %q, %q, s.proxyClient, s.db): %d, %v", modulePath, version, code, err), code
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
		log.Printf("db.RefreshSearchDocuments(ctx): %v", err)
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
		log.Printf("Error inserting index versions: %v", err)
		http.Error(w, "error inserting versions", http.StatusInternalServerError)
		return
	}
	for _, version := range versions {
		if err := s.queue.ScheduleFetch(appengine.NewContext(r), version.Path, version.Version); err != nil {
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
		log.Printf("Error getting versions to fetch: %v", err)
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

// handleStatusPage serves the cron status page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	const pageSize = 20
	next, err := s.db.GetNextVersionsToFetch(ctx, pageSize)
	if err != nil {
		log.Printf("Error fetching next versions: %v", err)
		http.Error(w, "error fetching next versions", http.StatusInternalServerError)
		return
	}
	failures, err := s.db.GetRecentFailedVersions(ctx, pageSize)
	if err != nil {
		log.Printf("Error fetching recent failures: %v", err)
		http.Error(w, "error fetching recent failures", http.StatusInternalServerError)
		return
	}
	recents, err := s.db.GetRecentVersions(ctx, pageSize)
	if err != nil {
		log.Printf("Error fetching recent versions")
		http.Error(w, "error fetching recent versions", http.StatusInternalServerError)
		return
	}
	stats, err := s.db.GetVersionStats(ctx)
	if err != nil {
		log.Printf("Error fetching stats: %v", err)
		http.Error(w, "error fetching stats", http.StatusInternalServerError)
		return
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
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "error rendering template", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(w, &buf); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("Error copying buffer to ResponseWriter: %v", err)
	}
}

func (s *Server) handlePopulateStdLib(w http.ResponseWriter, r *http.Request) {
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
	stdlibBetaVersions := []string{"v1.13.0-beta1"}

	var versionsToQueue [][]string
	for majMin, maxPatch := range stdlibVersions {
		for patch := 0; patch <= maxPatch; patch++ {
			v := fmt.Sprintf("%s.%d", majMin, patch)
			versionsToQueue = append(versionsToQueue, []string{"std", v})
			if majMin == "v1.13" {
				// Starting in go1.13, "cmd" becomes a nested
				// module and needs to be fetched separately.
				versionsToQueue = append(versionsToQueue, []string{"cmd", v})
			}
		}
	}
	for _, betaVersion := range stdlibBetaVersions {
		versionsToQueue = append(versionsToQueue,
			[]string{"std", betaVersion},
			[]string{"cmd", betaVersion})
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
