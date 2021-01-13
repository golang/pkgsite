// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"html"
	"net/http"

	"golang.org/x/pkgsite/internal/cookie"
	"golang.org/x/pkgsite/internal/log"
)

const (
	// RedirectedFromPlaceholder should be used as the value for any requests
	// redirected from a different pkg.go.dev path.
	// It is substituted for the actual value by the RedirectedFrom middleware.
	RedirectedFromPathPlaceholder = "$$GODISCOVERY_REDIRECTEDFROMPATH$$"

	// RedirectedFromPlaceholder should be used as the value for any requests
	// redirected from a different pkg.go.dev path.
	// It is substituted for the actual value by the RedirectedFrom middleware.
	RedirectedFromClassPlaceholder = "$$GODISCOVERY_REDIRECTEDFROMCLASS$$"
)

// RedirectedFrom adds a corresponding redirected from value to the rendered
// page if the request is due to a redirect from another unit page.
func RedirectedFrom() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			path, err := cookie.Extract(w, r, cookie.AlternativeModuleFlash)
			if err != nil {
				log.Errorf(ctx, "GetFlashMessage(w, r, %q): %v", err)
			}
			var class string
			if path == "" {
				class = "UnitHeader-redirectedFromBanner--none"
			}

			crw := &capturingResponseWriter{ResponseWriter: w}
			h.ServeHTTP(crw, r)
			body := crw.bytes()
			body = bytes.ReplaceAll(body, []byte(RedirectedFromClassPlaceholder), []byte(class))
			body = bytes.ReplaceAll(body, []byte(RedirectedFromPathPlaceholder), []byte(html.EscapeString(path)))
			if _, err := w.Write(body); err != nil {
				log.Errorf(r.Context(), "RedirectedFrom, writing: %v", err)
			}
		})
	}
}
