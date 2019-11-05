// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"

	"cloud.google.com/go/errorreporting"
)

// ErrorReporting returns a middleware that reports any server errors using the
// report func.
func ErrorReporting(report func(errorreporting.Entry)) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w2 := &responseWriter{ResponseWriter: w}
			h.ServeHTTP(w2, r)
			if w2.status >= 500 {
				e := errorreporting.Entry{
					Error: fmt.Errorf("handler for %q returned status code %d", r.URL.Path, w2.status),
					Req:   r,
				}
				report(e)
			}
		})
	}
}
