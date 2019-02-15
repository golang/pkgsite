// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"bytes"
	"encoding/json"
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

// FetchVersion makes a request to url for the given module version.
func (c *Client) FetchVersion(name, version string) error {
	data := map[string]string{
		"name":    name,
		"version": version,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		return err
	}

	contentType := "application/json"
	r, err := http.Post(c.url, contentType, &buf)
	if err != nil {
		return fmt.Errorf("http.Post(%q, %q, %q) error: %v", c.url, contentType, data, err)
	}

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("http.Post(%q, %q, %q) returned response: %d (%q)",
			c.url, contentType, data, r.StatusCode, r.Status)
	}
	return nil
}
