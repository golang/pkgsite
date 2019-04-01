// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

// clientTimeout bounds requests to the fetch service.  This is done
// independent of the request context, as the fetch service is expected to be
// relatively low-latency.
const clientTimeout = 1 * time.Minute

// A Client is used to communicate with the discovery fetch service.
type Client struct {
	*http.Client
	url string // URL of the fetch service
}

// New constructs a *Client using the provided url, which is expected to be an
// absolute URI to a fetch service that can be directly passed to http.Get.
func New(url string) *Client {
	return &Client{
		Client: &http.Client{Timeout: clientTimeout},
		url:    url,
	}
}

// FetchVersion makes a request for the module with name and version.
func (c *Client) FetchVersion(ctx context.Context, name, version string) error {
	url := fmt.Sprintf("%s/%s/@v/%s", c.url, name, version)
	resp, err := ctxhttp.Get(ctx, c.Client, url)
	if err != nil {
		return fmt.Errorf("ctxhttp.Get(ctx, c.Client, %q): %v", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ctxhttp.Get(ctx, c.Client, %q) returned response: %d (%q)", url, resp.StatusCode, resp.Status)
	}
	return nil
}
