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
	"sync"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
)

// Server is an http.Handler that implements functionality for managing the
// processing of new module versions.
type Server struct {
	http.Handler

	indexClient *index.Client
	fetchClient *fetch.Client
	db          *postgres.DB

	workers       int
	indexTemplate *template.Template
}

// NewServer creates a new Server with the given dependencies.
func NewServer(db *postgres.DB, indexClient *index.Client, fetchClient *fetch.Client, indexTemplate *template.Template, workers int) *Server {
	mux := http.NewServeMux()
	s := &Server{
		Handler:     mux,
		db:          db,
		indexClient: indexClient,
		fetchClient: fetchClient,

		workers:       workers,
		indexTemplate: indexTemplate,
	}
	mux.HandleFunc("/new/", s.handleNewVersions)
	mux.HandleFunc("/retry/", s.handleRetryVersions)
	mux.HandleFunc("/indexupdate/", s.handleIndexUpdate)
	mux.HandleFunc("/fetchversions/", s.handleFetchVersions)
	mux.HandleFunc("/", s.handleIndex)
	return s
}

// handleNewVersions fetches new versions from the module index and fetches
// them.
func (s *Server) handleNewVersions(w http.ResponseWriter, r *http.Request) {
	logs, err := fetchAndStoreVersions(r.Context(), s.indexClient, s.db)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("FetchAndStoreVersions(indexClient, db): %v", err)
		return
	}
	log.Printf("Fetching %d versions", len(logs))

	fetchVersions(r.Context(), s.fetchClient, logs, s.workers)
	fmt.Fprint(w, fmt.Sprintf("Requested %d new versions!", len(logs)))
}

// handleRetryVersions retries versions in the version_logs table that have
// errors.
func (s *Server) handleRetryVersions(w http.ResponseWriter, r *http.Request) {
	logs, err := s.db.GetVersionsToRetry(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("db.GetVersionsToRetry(ctx): %v", err)
		return
	}
	log.Printf("Fetching %d versions", len(logs))

	fetchVersions(r.Context(), s.fetchClient, logs, s.workers)
	fmt.Fprint(w, fmt.Sprintf("Requested %d versions!", len(logs)))
}

// handleIndexUpdate fetches new versions from the module index and inserts
// them into the module_version_states table, but does not perform a fetch.
func (s *Server) handleIndexUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limitParam := r.FormValue("limit")
	var (
		limit = 10
		err   error
	)
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	since, err := s.db.LatestIndexTimestamp(ctx)
	if err != nil {
		log.Printf("error doing proxy index update: %v", err)
		http.Error(w, "error doing proxy index update", http.StatusInternalServerError)
		return
	}
	versions, err := s.indexClient.GetIndexVersions(ctx, since, limit)
	if err != nil {
		log.Printf("error getting index versions: %v", err)
		http.Error(w, "error getting versions", http.StatusInternalServerError)
		return
	}
	if err := s.db.InsertIndexVersions(ctx, versions); err != nil {
		log.Printf("error inserting index versions: %v", err)
		http.Error(w, "error inserting versions", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		fmt.Fprintf(w, "inserted %s@%s\n", v.Path, v.Version)
	}
}

// computeBackoff computes the duration of time to wait before next attempting
// to fetch the given version.
func computeBackoff(state *internal.VersionState, resp *fetch.Response) time.Duration {
	const (
		minBackoff    = time.Minute
		backOffFactor = 2
	)
	if state.LastProcessedAt == nil {
		return minBackoff
	}
	return backOffFactor * state.NextProcessedAfter.Sub(*state.LastProcessedAt)
}

// handleFetchVersions queries the module_version_states table for the next
// batch of module versions to process, and calls the fetch service.
func (s *Server) handleFetchVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limitParam := r.FormValue("limit")
	var (
		limit = 10
		err   error
	)
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			log.Printf("error parsing limit parameter: %v", err)
			limit = 10
		}
	}
	versions, err := s.db.GetNextVersionsToFetch(ctx, limit)
	if err != nil {
		log.Printf("error getting versions to fetch: %v", err)
		http.Error(w, "error getting versions to fetch", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	var requests []*fetch.Request
	versionLookup := make(map[fetch.Request]*internal.VersionState)
	for _, v := range versions {
		request := fetch.Request{ModulePath: v.ModulePath, Version: v.Version}
		versionLookup[request] = v
		requests = append(requests, &request)
	}
	responses := make(chan *fetch.Response, 10)
	go fetchIndexVersions(ctx, s.fetchClient, requests, s.workers, responses)
	var wg sync.WaitGroup
	for resp := range responses {
		wg.Add(1)
		go func(resp *fetch.Response) {
			defer wg.Done()
			v, ok := versionLookup[resp.Request]
			if !ok {
				log.Printf("BUG: response not found in requests")
				return
			}
			backOff := computeBackoff(v, resp)
			if err := s.db.UpdateVersionState(ctx, resp.ModulePath, resp.Version, resp.StatusCode, resp.Error, backOff); err != nil {
				log.Printf("db.SetVersionState(): %v", err)
			}
			fmt.Fprintf(w, "got %d for %s@%s:%s\n", resp.StatusCode, resp.ModulePath, resp.Version, resp.Error)
		}(resp)
	}
	wg.Wait()
}

// handleIndex serves the cron status page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	next, err := s.db.GetNextVersionsToFetch(r.Context(), 100)
	if err != nil {
		log.Printf("error fetching versions: %v", err)
		http.Error(w, "error fetching versions", http.StatusInternalServerError)
		return
	}
	failures, err := s.db.GetRecentFailedVersions(r.Context(), 100)
	if err != nil {
		log.Printf("error fetching recent failures: %v", err)
		http.Error(w, "error fetching recent failures", http.StatusInternalServerError)
		return
	}
	page := struct {
		Next, RecentFailures []*internal.VersionState
	}{
		next, failures,
	}
	var buf bytes.Buffer
	if err := s.indexTemplate.Execute(&buf, page); err != nil {
		log.Printf("error rendering template: %v", err)
		http.Error(w, "error rendering template", http.StatusInternalServerError)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("Error copying buffer to ResponseWriter: %v", err)
	}
}
