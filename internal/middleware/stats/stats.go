// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"context"
	"encoding/json"
	"hash"
	"hash/fnv"
	"net/http"
	"time"
)

// statsKey is the type of the context key for stats.
type statsKey struct{}

// Stats returns a Middleware that, instead of serving the page,
// serves statistics about the page.
func Stats() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := newStatsResponseWriter()
			ctx := context.WithValue(r.Context(), statsKey{}, sw.stats.Other)
			h.ServeHTTP(sw, r.WithContext(ctx))
			sw.WriteStats(ctx, w)
		})
	}
}

// set sets a stat named key in the current context. If key already has a
// value, the old and new value are both stored in a slice.
func set(ctx context.Context, key string, value any) {
	x := ctx.Value(statsKey{})
	if x == nil {
		return
	}
	m := x.(map[string]any)
	v, ok := m[key]
	if !ok {
		m[key] = value
	} else if s, ok := v.([]any); ok {
		m[key] = append(s, value)
	} else {
		m[key] = []any{v, value}
	}
}

// Elapsed records as a stat the elapsed time for a
// function execution. Invoke like so:
//
//	defer Elapsed(ctx, "FunctionName")()
//
// The resulting stat will be called "FunctionName ms" and will
// be the wall-clock execution time of the function in milliseconds.
func Elapsed(ctx context.Context, name string) func() {
	start := time.Now()
	return func() {
		set(ctx, name+" ms", time.Since(start).Milliseconds())
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
	Other             map[string]any
}

func newStatsResponseWriter() *statsResponseWriter {
	return &statsResponseWriter{
		header: http.Header{},
		start:  time.Now(),
		hasher: fnv.New64a(),
		stats:  PageStats{Other: map[string]any{}},
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
