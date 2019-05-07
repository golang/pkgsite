// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"fmt"
	"io/ioutil"
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

// Request represents a request to the fetch service.
type Request struct {
	ModulePath, Version string
}

// Response represents a response from the fetch service.
type Response struct {
	Request
	StatusCode int
	Error      string
}

// FetchVersion makes a request for the module with name and version.
func (c *Client) FetchVersion(ctx context.Context, request *Request) *Response {
	url := fmt.Sprintf("%s/%s/@v/%s", c.url, request.ModulePath, request.Version)
	resp, err := ctxhttp.Get(ctx, c.Client, url)
	if err != nil {
		// treat an http error as a 503
		return &Response{
			Request:    *request,
			StatusCode: http.StatusServiceUnavailable,
			Error:      fmt.Sprintf("ctxhttp.Get(ctx, c.Client, %q): %v", url, err),
		}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return &Response{Request: *request, StatusCode: resp.StatusCode}
	case resp.StatusCode >= 400 && resp.StatusCode < 600:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return &Response{
				Request:    *request,
				StatusCode: resp.StatusCode,
				Error:      fmt.Sprintf("error reading response body: %v", err),
			}
		}
		return &Response{
			Request:    *request,
			StatusCode: resp.StatusCode,
			Error:      string(body),
		}
	default:
		return &Response{
			Request:    *request,
			StatusCode: http.StatusInternalServerError,
			Error:      fmt.Sprintf("fetch service return invalid status code %d", resp.StatusCode),
		}
	}
}
