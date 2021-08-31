// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/pkgsite/internal/auth"
	"golang.org/x/pkgsite/internal/derrors"
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
func (c *Client) GetVersions(pkgPath string) (_ *VersionsDetails, err error) {
	defer derrors.Wrap(&err, "GetVersions(%q)", pkgPath)
	u := fmt.Sprintf("%s/%s?tab=versions&content=json", c.url, pkgPath)
	r, err := c.httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(r.Status)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var vd VersionsDetails
	if err := json.Unmarshal(body, &vd); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v:\nDoes GO_DISCOVERY_SERVE_STATS=true on the frontend?", err)
	}
	return &vd, nil
}
