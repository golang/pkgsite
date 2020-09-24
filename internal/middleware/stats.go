// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"encoding/json"
	"hash"
	"hash/fnv"
	"net/http"
	"time"
)

// Stats returns a Middleware that, instead of serving the page,
// serves statistics about the page.
func Stats() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := newStatsResponseWriter()
			h.ServeHTTP(sw, r)
			sw.WriteStats(r.Context(), w)
		})
	}
}

// statsResponseWriter is an http.ResponseWriter that tracks statistics about
// the page being written.
type statsResponseWriter struct {
	header http.Header // required for a ResponseWriter; ignored
	start  time.Time   // start time of request
	hasher hash.Hash64
	stats  PageStats
}

type PageStats struct {
	MillisToFirstByte int64
	MillisToLastByte  int64
	Hash              uint64 // hash of page contents
	Size              int    // total size of data written
	StatusCode        int    // HTTP status
}

func newStatsResponseWriter() *statsResponseWriter {
	return &statsResponseWriter{
		header: http.Header{},
		start:  time.Now(),
		hasher: fnv.New64a(),
	}
}

// Header implements http.ResponseWriter.Header.
func (s *statsResponseWriter) Header() http.Header { return s.header }

// WriteHeader implements http.ResponseWriter.WriteHeader.
func (s *statsResponseWriter) WriteHeader(statusCode int) {
	s.stats.StatusCode = statusCode
}

// Write implements http.ResponseWriter.Write by
// tracking statistics about the data being written.
func (s *statsResponseWriter) Write(data []byte) (int, error) {
	if s.stats.Size == 0 {
		s.stats.MillisToFirstByte = time.Since(s.start).Milliseconds()
	}
	if s.stats.StatusCode == 0 {
		s.WriteHeader(http.StatusOK)
	}
	s.stats.Size += len(data)
	s.hasher.Write(data)
	return len(data), nil
}

// WriteStats writes the statistics to w.
func (s *statsResponseWriter) WriteStats(ctx context.Context, w http.ResponseWriter) {
	s.stats.MillisToLastByte = time.Since(s.start).Milliseconds()
	s.stats.Hash = s.hasher.Sum64()
	data, err := json.Marshal(s.stats)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		_, _ = w.Write(data)
	}
}
