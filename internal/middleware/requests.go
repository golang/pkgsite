// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"sort"
	"sync"

	"golang.org/x/pkgsite/internal"
)

var (
	requestMapMu sync.Mutex
	requestMap   = map[string]*internal.RequestInfo{}
)

// RequestInfo adds information about the request to a context.
// It also stores it while the request is active.
// [ActiveRequests] retrieves all stored requests.
func RequestInfo() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ri := internal.NewRequestInfo(r)
			ctx, cancel := context.WithCancelCause(r.Context())
			ri.Cancel = cancel

			// If the request has a trace ID, store it in the requestMap while
			// it is active.
			if ri.TraceID != "" {
				requestMapMu.Lock()
				requestMap[ri.TraceID] = ri
				requestMapMu.Unlock()

				defer func() {
					requestMapMu.Lock()
					delete(requestMap, ri.TraceID)
					requestMapMu.Unlock()
				}()
			}

			ctx = internal.NewContextWithRequestInfo(ctx, ri)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ActiveRequests returns all requests that are currently being handled by the server,
// sorted by start time.
func ActiveRequests() []*internal.RequestInfo {
	requestMapMu.Lock()
	defer requestMapMu.Unlock()
	var ris []*internal.RequestInfo
	for _, ri := range requestMap {
		ris = append(ris, ri)
	}
	sort.Slice(ris, func(i, j int) bool { return ris[i].Start.Before(ris[j].Start) })
	return ris
}

// RequestForTraceID returns the active request with the given trace ID,
// or nil if there is no such request.
func RequestForTraceID(id string) *internal.RequestInfo {
	requestMapMu.Lock()
	defer requestMapMu.Unlock()
	return requestMap[id]
}
