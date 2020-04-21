// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package auth authorizes programs to make HTTP requests to the discovery site.
package auth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/pkgsite/internal/derrors"
)

// clientID is the OAuth 2.0 Client ID.
// See https://cloud.google.com/iap/docs/authentication-howto for more details.
const clientID = "117187402928-nl3u0qo5l2c2hhsuf2qj8irsfb3l6hfc.apps.googleusercontent.com"

// NewClient creates an http.Client for accessing go-discovery services.
// Its argument is the JSON contents of a service account credentials file.
func NewClient(jsonCreds []byte) (_ *http.Client, err error) {
	defer derrors.Wrap(&err, "auth.NewClient(jsonCreds)")

	ts, err := TokenSource(jsonCreds)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(context.Background(), ts), nil
}

// TokenSource creates an oauth2.TokenSource for accessing go-discovery services.
// Its argument is the JSON contents of a service account credentials file.
func TokenSource(json []byte) (_ oauth2.TokenSource, err error) {
	defer derrors.Wrap(&err, "auth.TokenSource(jsonCreds)")
	config, err := google.JWTConfigFromJSON(json)
	if err != nil {
		return nil, fmt.Errorf("JWTConfigFromJSON: %v", err)
	}
	config.PrivateClaims = map[string]interface{}{"target_audience": clientID}
	// This is required: the docstring says "specifies whether ID token should be
	// used instead of access token when the server returns both", but the
	// implementation says differently.
	config.UseIDToken = true
	// Use the background context, in case the token source stores and re-uses it
	// to refresh the token from a server.
	return config.TokenSource(context.Background()), nil
}

// Header returns a header value (typically a Bearer token) to be used in the
// HTTP 'Authorization' header.
func Header(jsonCreds []byte) (_ string, err error) {
	defer derrors.Wrap(&err, "auth.Header(jsonCreds)")
	ts, err := TokenSource(jsonCreds)
	if err != nil {
		return "", err
	}
	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("TokenSource.Token(): %v", err)
	}
	// This is a dummy request to get the authorization header.
	req, err := http.NewRequest("GET", "http://localhost", nil)
	if err != nil {
		return "", fmt.Errorf("http.NewRequest(): %v", err)
	}
	token.SetAuthHeader(req)
	return req.Header.Get("Authorization"), nil
}
