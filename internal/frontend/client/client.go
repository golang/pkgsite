// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package client provides a client for interacting with the frontend.
// It is only used for tests.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"golang.org/x/pkgsite/internal/auth"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/frontend/versions"
)

// A Client for interacting with the frontend. This is only used for tests.
type Client struct {
	// URL of the frontend server host.
	url string

	// Client used for HTTP requests.
	httpClient *http.Client
}

// New creates a new frontend client. This is only used for tests.
func New(url string) *Client {
	tok, ok := os.LookupEnv("GO_DISCOVERY_FRONTEND_AUTHORIZATION")
	c := &Client{
		url:        url,
		httpClient: http.DefaultClient,
	}
	if ok {
		c.httpClient = auth.NewClientBearer(tok)
	}
	return c
}

// GetVersions returns a VersionsDetails for the specified pkgPath.
// This is only used for tests.
func (c *Client) GetVersions(pkgPath string) (_ *versions.VersionsDetails, err error) {
	defer derrors.Wrap(&err, "GetVersions(%q)", pkgPath)
	u := fmt.Sprintf("%s/%s?tab=versions&content=json", c.url, pkgPath)
	body, err := c.fetchJSONPage(u)
	if err != nil {
		return nil, err
	}
	var vd versions.VersionsDetails
	if err := json.Unmarshal(body, &vd); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v:\nDoes GO_DISCOVERY_SERVE_STATS=true on the frontend?", err)
	}
	return &vd, nil
}

// Search returns a SearchPage for a search query and mode.
func (c *Client) Search(q, mode string) (_ *frontend.SearchPage, err error) {
	defer derrors.Wrap(&err, "Search(%q, %q)", q, mode)
	u := fmt.Sprintf("%s/search?q=%s&content=json&m=%s", c.url, url.QueryEscape(q), mode)
	body, err := c.fetchJSONPage(u)
	if err != nil {
		return nil, err
	}
	var sp frontend.SearchPage
	if err := json.Unmarshal(body, &sp); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v:\nDoes GO_DISCOVERY_SERVE_STATS=true on the frontend?", err)
	}
	return &sp, nil
}

func (c *Client) fetchJSONPage(url string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "fetchJSONPage(%q)", url)
	r, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, errors.New(r.Status)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
