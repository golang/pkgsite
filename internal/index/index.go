// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package index provides a client for communicating with the module index.
package index

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// A Client is used by the worker service to communicate with the module index.
type Client struct {
	// URL of the module index
	url string

	// client used for HTTP requests. It is mutable for testing purposes.
	httpClient *http.Client
}

// New constructs a *Client using the provided rawurl, which is expected to
// be an absolute URI that can be directly passed to http.Get.
func New(rawurl string) (_ *Client, err error) {
	defer derrors.Add(&err, "index.New(%q)", rawurl)

	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, fmt.Errorf("url.Parse(%q): %v", rawurl, err)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost") {
		return nil, fmt.Errorf("scheme must be https (got %s)", u.Scheme)
	}
	return &Client{url: strings.TrimRight(rawurl, "/"), httpClient: &http.Client{Transport: &ochttp.Transport{}}}, nil
}

func (c *Client) pollURL(since time.Time, limit int) string {
	values := url.Values{}
	values.Set("since", since.Format(time.RFC3339))
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	return fmt.Sprintf("%s?%s", c.url, values.Encode())
}

// GetVersions queries the index for new versions.
func (c *Client) GetVersions(ctx context.Context, since time.Time, limit int) (_ []*internal.IndexVersion, err error) {
	defer derrors.Wrap(&err, "index.Client.GetVersions(ctx, %s, %d)", since, limit)

	u := c.pollURL(since, limit)
	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, nil, %q): %v", u, err)
	}
	defer r.Body.Close()

	var versions []*internal.IndexVersion
	dec := json.NewDecoder(r.Body)

	// The module index returns a stream of JSON objects formatted with newline
	// as the delimiter.
	for dec.More() {
		var l internal.IndexVersion
		if err := dec.Decode(&l); err != nil {
			return nil, fmt.Errorf("decoding JSON: %v", err)
		}
		versions = append(versions, &l)
	}
	return versions, nil
}
