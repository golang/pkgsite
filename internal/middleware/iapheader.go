// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/api/idtoken"
)

// ValidateIAPHeader checks that the request has a header that proves it arrived
// via the IAP.
// See https://cloud.google.com/iap/docs/signed-headers-howto#securing_iap_headers.
func ValidateIAPHeader(audience string) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health checks don't come from the IAP; allow them.
			if r.URL.Path != "/healthz" {
				// Adapted from https://github.com/GoogleCloudPlatform/golang-samples/blob/master/iap/validate.go
				token := r.Header.Get("X-Goog-IAP-JWT-Assertion")
				if err := validateIAPToken(r.Context(), token, audience); err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
			}
			h.ServeHTTP(w, r)
		})
	}
}

func validateIAPToken(ctx context.Context, iapJWT, audience string) error {
	if iapJWT == "" {
		return errors.New("missing IAP token")
	}
	if _, err := idtoken.Validate(ctx, iapJWT, audience); err != nil {
		return fmt.Errorf("validating IPA token: %v", err)
	}
	return nil
}
