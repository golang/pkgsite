// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/fetch"
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
	mux := http.NewServeMux()
	s := &Server{
		Handler:     mux,
		db:          db,
		indexClient: indexClient,
		proxyClient: proxyClient,
		queue:       queue,

		indexTemplate: indexTemplate,
	}
	mux.HandleFunc("/poll-and-queue/", s.handleIndexAndQueue)
	mux.HandleFunc("/requeue/", s.handleRequeue)
	mux.HandleFunc("/refresh-search/", s.handleRefreshSearch)
	mux.Handle("/fetch/", http.StripPrefix("/fetch", http.HandlerFunc(s.handleFetch)))
	mux.HandleFunc("/", s.handleStatusPage)
	return s
}

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

	modulePath, version, err := fetch.ParseModulePathAndVersion(r.URL.Path)
	if err != nil {
		log.Printf("fetch.ParseModulePathAndVersion(%q): %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	code, err := fetchAndUpdateState(r.Context(), modulePath, version, s.proxyClient, s.db)
	if err != nil {
		http.Error(w, http.StatusText(code), code)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, "Downloaded %s@%s\n", modulePath, version)
	log.Printf("Downloaded: %q %q", modulePath, version)
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
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("Error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	versions, err := s.db.GetNextVersionsToFetch(ctx, limit)
	if err != nil {
		log.Printf("Error getting versions to fetch: %v", err)
		http.Error(w, "error getting versions to fetch", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		s.queue.ScheduleFetch(ctx, v.ModulePath, v.Version)
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
