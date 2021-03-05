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
	// From content/static/html/base.tmpl
	"'sha256-CgM7SjnSbDyuIteS+D1CQuSnzyKwL0qtXLU6ZW2hB+g='",
	"'sha256-dwce5DnVX7uk6fdvvNxQyLTH/cJrTMDK6zzrdKwdwcg='",
	"'sha256-M35cNZ8vPcaBGw5WTgh0Gn7DLsxkvPbdTFN1pELeevM='",
	// From content/static/html/pages/badge.tmpl
	"'sha256-v9+UvX+P27rKraeTl7uAfOWdLmmQU39RskIoqUrU4wo='",
	// From content/static/html/pages/fetch.tmpl
	"'sha256-4FhQmh9Hu76JzYm35KNNysU2Z7buJwg3cMSHsGwKSCE='",
	// From content/static/html/worker/index.tmpl
	"'sha256-y5EX2GR3tCwSK0/kmqZnsWVeBROA8tA75L+I+woljOE='",
	// From content/static/html/pages/unit.tmpl
	"'sha256-hsHIJwO1h0Vzwa75j0l07kUfQ7MEZGI/HlSPB/8leZ0='",
	// From content/static/html/pages/unit_details.tmpl
	"'sha256-mKoRSTy/ibLlGchw5uTQdCsNmqX/Lt9zNnBMtD8u5Us='",
	"'sha256-/Mz1Pw/2FXZF9LLUgqsWFCN718K+/UPib6w8IBIf6Xk='",
	"'sha256-L+G1K2BEWa+o2vPy1pwdabLjINBByPWi1NkRwvASUq8='",
	"'sha256-hb8VdkRSeBmkNlbshYmBnkYWC/BYHCPiz5s7liRcZNM='",
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
