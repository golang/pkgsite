// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/pkgsite/internal/log"
)

// NoncePlaceholder should be used as the value for nonces in rendered content.
// It is substituted for the actual nonce value by the SecureHeaders middleware.
const NoncePlaceholder = "$$GODISCOVERYNONCE$$"

// SecureHeaders adds a content-security-policy and other security-related
// headers to all responses.
func SecureHeaders() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nonce, err := generateNonce()
			if err != nil {
				log.Infof(r.Context(), "generateNonce(): %v", err)
			}
			csp := []string{
				// Disallow plugin content: pkg.go.dev does not use it.
				"object-src 'none'",
				// Disallow <base> URIs, which prevents attackers from changing the
				// locations of scripts loaded from relative URLs. The site doesn’t have
				// a <base> tag anyway.
				"base-uri 'none'",

				fmt.Sprintf("script-src 'nonce-%s' 'unsafe-inline' 'strict-dynamic' https: http:",
					nonce),
			}
			w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			crw := &capturingResponseWriter{ResponseWriter: w}
			h.ServeHTTP(crw, r)
			body := bytes.ReplaceAll(crw.bytes(), []byte(NoncePlaceholder), []byte(nonce))
			if _, err := w.Write(body); err != nil {
				log.Errorf(r.Context(), "SecureHeaders, writing: %v", err)
			}
		})
	}
}

// capturingResponseWriter is an http.ResponseWriter that captures
// the body for later processing.
type capturingResponseWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (c *capturingResponseWriter) Write(b []byte) (int, error) {
	return c.buf.Write(b)
}

func (c *capturingResponseWriter) bytes() []byte {
	return c.buf.Bytes()
}
