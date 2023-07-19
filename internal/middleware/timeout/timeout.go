// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package timeout

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns a new Middleware that times out each request after the given
// duration.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
