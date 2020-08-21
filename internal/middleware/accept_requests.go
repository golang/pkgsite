// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
)

const maxURILength = 1000

// AcceptRequests serves 405 (Method Not Allowed) for any method not on the
// given list and 414 (Method Request URI Too Long) for any URI that exceeds
// the maxURILength.
func AcceptRequests(methods ...string) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.URL.String()) >= maxURILength {
				http.Error(w, http.StatusText(http.StatusRequestURITooLong), http.StatusRequestURITooLong)
				return
			}
			for _, m := range methods {
				if r.Method == m {
					h.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		})
	}
}
