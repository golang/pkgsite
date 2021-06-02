// Copyright 2019-2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

var scriptHashes = []string{
	// From content/static/base/base.tmpl
	"'sha256-CoGrkqEM1Kjjf5b1bpcnDLl8ZZLAsVX+BoAzZ5+AOmc='",
	"'sha256-karKh1IrXOF1g+uoSxK+k9BuciCwYY/ytGuQVUiRzcM='",
	"'sha256-2owiLItzX793qLVobnd0z9Z90a2wej4/C73Qb0qNtO4='",
	// From content/static/badge/badge.tmpl
	"'sha256-lYbH/hg0O4Oc1stV4ysso2zlUCXY0yGRUZ5zDOnZ9hI='",
	// From content/static/fetch/fetch.tmpl
	"'sha256-NL8cRfvzPNDO6ZYKQYWS1kPknRV9gUCJoTk+fRR04zg='",
	// From content/static/main-layout/main-layout.tmpl
	"'sha256-cVUOUH4vbDOJY6vJAjvJj3d2jAmDiwy/SyVsZgZVxVM='",
	// From content/static/html/pages/unit.tmpl
	"'sha256-r4g06j/B7WYKOSl8cFfvuZOyiYA1tOyrbnxapiSP64g='",
	// From content/static/html/pages/unit_details.tmpl
	"'sha256-nF5UdhqQFxB95DCaw1XdSQCEkIjoMhorTCQ+nQ4+Lq4='",
	"'sha256-L+G1K2BEWa+o2vPy1pwdabLjINBByPWi1NkRwvASUq8='",
	"'sha256-hb8VdkRSeBmkNlbshYmBnkYWC/BYHCPiz5s7liRcZNM='",
	// From content/static/html/pages/unit_versions.tmpl
	"'sha256-KBdPSv2Ajjw3jsa29qBhRW49nNx3jXxOLZIWX545FCA='",
	// From content/static/styleguide/styleguide.tmpl
	"'sha256-Z9STHpM3Fz5XojcH5dbUK50Igi6qInBbVVaqNpjL/HY='",
	// From content/static/html/worker/index.tmpl
	"'sha256-y5EX2GR3tCwSK0/kmqZnsWVeBROA8tA75L+I+woljOE='",
}

// SecureHeaders adds a content-security-policy and other security-related
// headers to all responses.
func SecureHeaders(enableCSP bool) Middleware {
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
			if enableCSP {
				w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))
			}
			// Don't allow frame embedding.
			w.Header().Set("X-Frame-Options", "deny")
			// Prevent MIME sniffing.
			w.Header().Set("X-Content-Type-Options", "nosniff")

			h.ServeHTTP(w, r)
		})
	}
}
