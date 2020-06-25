// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

var scriptHashes = []string{
	// From content/static/html/pages/base.tmpl
	"'sha256-d6W7MwuGWbguTHRzQhf5QN1jXmNo9Ao218saZkWLWZI='",
	"'sha256-CCu0fuIQFBHSCEpfR6ZRzzcczJIS/VGMGrez8LR49WY='",
	"'sha256-qPGTOKPn+niRiNKQIEX0Ktwuj+D+iPQWIxnlhPicw58='",
	// From content/static/html/pages/notfound.tmpl
	"'sha256-h5L4TV5GzTaBQYCnA8tDw9+9/AIdK9dwgkwlqFjVqEI='",
	// From content/static/html/pages/details.tmpl
	"'sha256-s16e7aT7Gsajq5UH1DbaEFEnNx2VjvS5Xixcxwm4+F8='",
	// From content/static/html/pages/pkg_doc.tmpl
	"'sha256-AvMTqQ+22BA0Nsht+ajju4EQseFQsoG1RxW3Nh6M+wc='",
	// From content/static/html/worker/index.tmpl
	"'sha256-5EpitFYSzGNQNUsqi5gAaLqnI3ZWfcRo/6gLTO0oCoE='",
}

// SecureHeaders adds a content-security-policy and other security-related
// headers to all responses.
func SecureHeaders() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			csp := []string{
				// Disallow plugin content: pkg.go.dev does not use it.
				"object-src 'none'",
				// Disallow <base> URIs, which prevents attackers from changing the
				// locations of scripts loaded from relative URLs. The site doesn’t have
				// a <base> tag anyway.
				"base-uri 'none'",
				fmt.Sprintf("script-src 'unsafe-inline' 'strict-dynamic' https: http: %s",
					strings.Join(scriptHashes, " ")),
			}
			w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			h.ServeHTTP(w, r)
		})
	}
}
