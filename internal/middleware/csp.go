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

// ContentSecurityPolicy adds a Content-Security-Policy header to all
// responses.
func ContentSecurityPolicy() Middleware {
	var p policy
	p.add("default-src", "'self'")
	p.add("style-src", "'self'", "fonts.googleapis.com")
	p.add("font-src", "'self'", "fonts.googleapis.com", "fonts.gstatic.com")
	p.add("img-src", "*")
	p.add("object-src 'none'")
	p.add("base-uri 'none'")
	csp := p.serialize()
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", csp)
			h.ServeHTTP(w, r)
		})
	}
}
