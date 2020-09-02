// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package auth authorizes programs to make HTTP requests to the discovery site.
package auth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal/derrors"
	"google.golang.org/api/idtoken"
)

// OAuth 2.0 Client IDs for the go-discovery (main) and the go-discovery-exp (exp) projects.
// See https://cloud.google.com/iap/docs/authentication-howto for more details.
const (
	mainClientID = "117187402928-nl3u0qo5l2c2hhsuf2qj8irsfb3l6hfc.apps.googleusercontent.com"
	expClientID  = "55665122702-tk2rogkaalgru7pqibvbltqs7geev8j5.apps.googleusercontent.com"
)

// NewClient creates an http.Client for accessing go-discovery services.
// Its first argument is the JSON contents of a service account credentials file.
// If nil, default credentials are used.
// Its second argument determines which client ID to use.
func NewClient(ctx context.Context, jsonCreds []byte, useExp bool) (_ *http.Client, err error) {
	defer derrors.Wrap(&err, "auth.NewClient(jsonCreds, %t)", useExp)
	audience, opts := idtokenArgs(jsonCreds, useExp)
	return idtoken.NewClient(ctx, audience, opts...)
}

// Header returns a header value (typically a Bearer token) to be used in the
// HTTP 'Authorization' header.
func Header(ctx context.Context, jsonCreds []byte, useExp bool) (_ string, err error) {
	defer derrors.Wrap(&err, "auth.Header(jsonCreds, %t)", useExp)

	audience, opts := idtokenArgs(jsonCreds, useExp)
	ts, err := idtoken.NewTokenSource(ctx, audience, opts...)
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

func idtokenArgs(jsonCreds []byte, useExp bool) (string, []idtoken.ClientOption) {
	var opts []idtoken.ClientOption
	if len(jsonCreds) > 0 {
		opts = append(opts, idtoken.WithCredentialsJSON(jsonCreds))
	}
	audience := mainClientID
	if useExp {
		audience = expClientID
	}
	return audience, opts
}
