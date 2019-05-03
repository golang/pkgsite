// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

var allowedExternalSrcs = []string{
	"fonts.googleapis.com",
	"fonts.gstatic.com",
}

// ContentSecurityPolicy adds a Content-Security-Policy header to all
// responses.
func ContentSecurityPolicy() Middleware {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("default-src 'self' %s", strings.Join(allowedExternalSrcs, " ")))
	b.WriteString("; object-src 'none'")
	b.WriteString("; base-uri 'none'")
	csp := b.String()
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", csp)
			h.ServeHTTP(w, r)
		})
	}
}
