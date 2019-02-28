// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"fmt"
	"net/http"
)

// A Client is used to communicate with the discovery fetch service.
type Client struct {
	url string // URL of the fetch service
}

// New constructs a *Client using the provided url, which is expected to be an
// absolute URI to a fetch service that can be directly passed to http.Get.
func New(url string) *Client {
	return &Client{
		url: url,
	}
}

// FetchVersion makes a request for the module with name and version.
func (c *Client) FetchVersion(name, version string) error {
	url := fmt.Sprintf("%s/%s/@v/%s", c.url, name, version)
	r, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http.Get(%q): %v", url, err)
	}

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("http.Get(%q) returned response: %d (%q)", url, r.StatusCode, r.Status)
	}
	return nil
}
