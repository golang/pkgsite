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

// policy is a helper for constructing content security policies.
type policy struct {
	directives []string
}

// add appends a new directive to the policy. Neither name nor values are
// validated, and no checking is performed that the new directive is unique.
func (p *policy) add(name string, values ...string) {
	d := name + " " + strings.Join(values, " ")
	p.directives = append(p.directives, d)
}

// serialize serializes the policy for use in the Content-Security-Policy
// header.
func (p *policy) serialize() string {
	return strings.Join(p.directives, "; ")
}

// SecureHeaders adds a content-security-policy and other security-related
// headers to all responses.
func SecureHeaders() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			useStrictDynamic := r.FormValue("csp-strict-dynamic") == "true"
			// These are special keywords within the CSP spec
			// https://www.w3.org/TR/CSP3/#framework-directive-source-list
			const (
				self = "'self'"
				none = "'none'"
			)

			var p policy

			// Set a strict fallback for content sources.
			p.add("default-src", self)

			// Allow known sources for fonts.
			p.add("font-src", self, "fonts.googleapis.com", "fonts.gstatic.com")

			// fonts.googleapis.com is used for fonts.
			p.add("style-src", self, "'unsafe-inline'", "fonts.googleapis.com")

			// Because we are rendering user-provided README's, we allow arbitrary image
			// sources. This could possibly be narrowed to known content hosts based on
			// e.g. the github.com CSP, but that seemed fragile.
			p.add("img-src", self, "data:", "*")

			// Disallow plugin content: the Discovery site does not use it.
			p.add("object-src", none)

			nonce, err := generateNonce()
			if err != nil {
				log.Infof(r.Context(), "generateNonce(): %v", err)
			}

			scriptSrcs := []string{
				fmt.Sprintf("'nonce-%s'", nonce),
				"www.gstatic.com",
				"www.googletagmanager.com",
				"support.google.com",
			}
			if useStrictDynamic {
				scriptSrcs = append(scriptSrcs, "'strict-dynamic'")
			}

			p.add("script-src", scriptSrcs...)

			// Don't allow framing.
			p.add("frame-ancestors", none)

			csp := p.serialize()
			w.Header().Set("Content-Security-Policy", csp)
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// TODO(b/144509703): avoid copying if possible
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
