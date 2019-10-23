// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal/log"
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

			// Because we are using the go/feedback widget, we need
			// to allow unsafe-inline and the following as valid
			// sources for stylesheets:
			// feedback.googleusercontent.com, www.gstatic.com.
			//
			// fonts.googleapis.com is used for fonts.
			p.add("style-src",
				self,
				"'unsafe-inline'",
				"fonts.googleapis.com",
				"feedback.googleusercontent.com",
				"www.gstatic.com")

			// Because we are using the go/feedback widget, we need to
			// allow the following as valid sources for our frame-src policy:
			// www.google.com, feedback.googleusercontent.com
			p.add("frame-src", self, "www.google.com", "feedback.googleusercontent.com")

			// Because we are rendering user-provided README's, we allow arbitrary image
			// sources. This could possibly be narrowed to known content hosts based on
			// e.g. the github.com CSP, but that seemed fragile.
			//
			// Because we are using the go/feedback widget, we need to specify
			// the data scheme data: to allow data: URIs to be uased as a
			// content source.
			p.add("img-src", self, "data:", "*")

			// Disallow plugin content: the Discovery site does not use it.
			p.add("object-src", none)

			nonce, err := generateNonce()
			if err != nil {
				log.Infof("generateNonce(): %v", err)
			}

			// Because we are using the go/feedback widget, we need to
			// allow "support.google.com" as a valid source for our script-src policy.
			//
			// www.google-analytics.com is needed for Google Analytics.
			p.add("script-src",
				fmt.Sprintf("'nonce-%s'", nonce),
				"www.gstatic.com",
				"support.google.com")

			// Don't allow framing.
			p.add("frame-ancestors", none)

			csp := p.serialize()
			w.Header().Set("Content-Security-Policy", csp)
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Replace the nonce in the page body.
			target := []byte(fmt.Sprintf("<script nonce=%q", NoncePlaceholder))
			replacement := []byte(fmt.Sprintf("<script nonce=%q", nonce))
			rrw := &replacingResponseWriter{
				ResponseWriter: w,
				target:         target,
				replacement:    replacement,
			}
			h.ServeHTTP(rrw, r)
			rrw.flush()
		})
	}
}

// replacingResponseWriter is an http.ResponseWriter that replaces
// target with replacement in the response body.
type replacingResponseWriter struct {
	http.ResponseWriter
	target, replacement []byte
	buf                 bytes.Buffer
}

func (r *replacingResponseWriter) Write(b []byte) (int, error) {
	return r.buf.Write(b)
}

func (r *replacingResponseWriter) flush() {
	data := r.buf.Bytes()
	data = bytes.ReplaceAll(data, r.target, r.replacement)
	if _, err := r.ResponseWriter.Write(data); err != nil {
		log.Errorf("replacingResponseWriter.flush: %v", err)
	}
}
