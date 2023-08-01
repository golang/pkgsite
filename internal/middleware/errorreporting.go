// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
)

// ErrorReporting returns a middleware that reports any server errors using the
// report func.
func ErrorReporting(reporter derrors.Reporter) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w2 := &erResponseWriter{ResponseWriter: w}
			h.ServeHTTP(w2, r)
			// Don't report if the bypass header was set.
			if w2.bypass {
				return
			}
			// Don't report success or client errors.
			if w2.status < 500 {
				return
			}
			// Don't report 503s; they are a normal consequence of load shedding.
			if w2.status == http.StatusServiceUnavailable {
				return
			}
			// Don't report errors where the proxy times out; they're too common.
			if w2.status == derrors.ToStatus(derrors.ProxyTimedOut) {
				return
			}
			// Don't report on proxy internal errors; they're not actionable.
			if w2.status == derrors.ToStatus(derrors.ProxyError) {
				return
			}
			// Don't report on vulndb errors.
			if w2.status == derrors.ToStatus(derrors.VulnDBError) {
				return
			}
			reporter.Report(
				fmt.Errorf("handler for %q returned status code %d", r.URL.Path, w2.status), r, nil)
		})
	}
}

type erResponseWriter struct {
	http.ResponseWriter

	bypass bool
	status int
}

func (rw *erResponseWriter) WriteHeader(code int) {
	rw.status = code
	if rw.ResponseWriter.Header().Get(config.BypassErrorReportingHeader) == "true" {
		rw.bypass = true
		// Don't send this header to clients.
		rw.ResponseWriter.Header().Del(config.BypassErrorReportingHeader)
	}
	rw.ResponseWriter.WriteHeader(code)
}
