// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"strings"
)

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
	p.add("style-src", self, "fonts.googleapis.com")
	p.add("font-src", self, "fonts.googleapis.com", "fonts.gstatic.com")

	// Because we are rendering user-provided README's, we allow arbitrary image
	// sources. This could possibly be narrowed to known content hosts based on
	// e.g. the github.com CSP, but that seemed fragile.
	p.add("img-src", "*")

	// Disallow plugin content: the Discovery site does not use it.
	p.add("object-src", none)

	// Don't allow document base URLs.
	p.add("base-uri", none)

	// Disallow JavaScript: the Discovery site does not use it. If down the road
	// we decide to use JavaScript, remove this and instead use a hash, or make
	// the header dynamic and incorporate a cryptographic nonce.
	p.add("script-src", none)

	// Don't allow framing.
	p.add("frame-ancestors", none)

	csp := p.serialize()

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", csp)
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			h.ServeHTTP(w, r)
		})
	}
}
