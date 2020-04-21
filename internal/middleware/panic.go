// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"

	"golang.org/x/pkgsite/internal/log"
)

// Panic returns a middleware that executes panicHandler on any panic
// originating from the delegate handler.
func Panic(panicHandler http.Handler) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if e := recover(); e != nil {
					log.Errorf(r.Context(), "middleware.Panic: %v", e)
					panicHandler.ServeHTTP(w, r)
				}
			}()
			h.ServeHTTP(w, r)
		})
	}
}
