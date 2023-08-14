// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"golang.org/x/pkgsite/internal/auth"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend/versions"
)

// A Client for interacting with the frontend. This is only used for tests.
type Client struct {
	// URL of the frontend server host.
	url string

	// Client used for HTTP requests.
	httpClient *http.Client
}

// NewClient creates a new frontend client. This is only used for tests.
func NewClient(url string) *Client {
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
func (c *Client) Search(q, mode string) (_ *SearchPage, err error) {
	defer derrors.Wrap(&err, "Search(%q, %q)", q, mode)
	u := fmt.Sprintf("%s/search?q=%s&content=json&m=%s", c.url, url.QueryEscape(q), mode)
	body, err := c.fetchJSONPage(u)
	if err != nil {
		return nil, err
	}
	var sp SearchPage
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
		return nil, fmt.Errorf(r.Status)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) shouldServeJSON(r *http.Request) bool {
	return s.serveStats && r.FormValue("content") == "json"
}

func (s *Server) serveJSONPage(w http.ResponseWriter, r *http.Request, d any) (err error) {
	defer derrors.Wrap(&err, "serveJSONPage(ctx, w, r)")
	if !s.shouldServeJSON(r) {
		return derrors.NotFound
	}
	data, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("json.Marshal: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("w.Write: %v", err)
	}
	return nil
}
