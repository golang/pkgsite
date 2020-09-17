// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxy provides a client for interacting with a proxy.
package proxy

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/mod/module"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// A Client is used by the fetch service to communicate with a module
// proxy. It handles all methods defined by go help goproxy.
type Client struct {
	// URL of the module proxy web server
	url string

	// client used for HTTP requests. It is mutable for testing purposes.
	httpClient *http.Client
}

// A VersionInfo contains metadata about a given version of a module.
type VersionInfo struct {
	Version string
	Time    time.Time
}

// New constructs a *Client using the provided url, which is expected to
// be an absolute URI that can be directly passed to http.Get.
func New(u string) (_ *Client, err error) {
	defer derrors.Wrap(&err, "proxy.New(%q)", u)
	return &Client{
		url:        strings.TrimRight(u, "/"),
		httpClient: &http.Client{Transport: &ochttp.Transport{}},
	}, nil
}

// GetInfo makes a request to $GOPROXY/<module>/@v/<requestedVersion>.info and
// transforms that data into a *VersionInfo.
func (c *Client) GetInfo(ctx context.Context, modulePath, requestedVersion string) (_ *VersionInfo, err error) {
	defer derrors.Wrap(&err, "proxy.Client.GetInfo(%q, %q)", modulePath, requestedVersion)
	data, err := c.readBody(ctx, modulePath, requestedVersion, "info")
	if err != nil {
		return nil, err
	}
	var v VersionInfo
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetMod makes a request to $GOPROXY/<module>/@v/<resolvedVersion>.mod and returns the raw data.
func (c *Client) GetMod(ctx context.Context, modulePath, resolvedVersion string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "proxy.Client.GetMod(%q, %q)", modulePath, resolvedVersion)
	return c.readBody(ctx, modulePath, resolvedVersion, "mod")
}

// GetZip makes a request to $GOPROXY/<modulePath>/@v/<resolvedVersion>.zip and
// transforms that data into a *zip.Reader. <resolvedVersion> must have already
// been resolved by first making a request to
// $GOPROXY/<modulePath>/@v/<requestedVersion>.info to obtained the valid
// semantic version.
func (c *Client) GetZip(ctx context.Context, modulePath, resolvedVersion string) (_ *zip.Reader, err error) {
	defer derrors.Wrap(&err, "proxy.Client.GetZip(ctx, %q, %q)", modulePath, resolvedVersion)

	bodyBytes, err := c.readBody(ctx, modulePath, resolvedVersion, "zip")
	if err != nil {
		return nil, err
	}
	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("zip.NewReader: %v: %w", err, derrors.BadModule)
	}
	return zipReader, nil
}

func (c *Client) escapedURL(modulePath, version, suffix string) (_ string, err error) {
	defer func() {
		derrors.Wrap(&err, "Client.escapedURL(%q, %q, %q)", modulePath, version, suffix)
	}()

	if suffix != "info" && suffix != "mod" && suffix != "zip" {
		return "", errors.New(`suffix must be "info", "mod" or "zip"`)
	}
	escapedPath, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("path: %v: %w", err, derrors.InvalidArgument)
	}
	if version == internal.LatestVersion {
		if suffix != "info" {
			return "", fmt.Errorf("cannot ask for latest with suffix %q", suffix)
		}
		return fmt.Sprintf("%s/%s/@latest", c.url, escapedPath), nil
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		return "", fmt.Errorf("version: %v: %w", err, derrors.InvalidArgument)
	}
	return fmt.Sprintf("%s/%s/@v/%s.%s", c.url, escapedPath, escapedVersion, suffix), nil
}

func (c *Client) readBody(ctx context.Context, modulePath, version, suffix string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "Client.readBody(%q, %q, %q)", modulePath, version, suffix)

	u, err := c.escapedURL(modulePath, version, suffix)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = c.executeRequest(ctx, u, func(body io.Reader) error {
		var err error
		data, err = ioutil.ReadAll(body)
		return err
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ListVersions makes a request to $GOPROXY/<path>/@v/list and returns the
// resulting version strings.
func (c *Client) ListVersions(ctx context.Context, modulePath string) ([]string, error) {
	escapedPath, err := module.EscapePath(modulePath)
	if err != nil {
		return nil, fmt.Errorf("module.EscapePath(%q): %w", modulePath, derrors.InvalidArgument)
	}
	u := fmt.Sprintf("%s/%s/@v/list", c.url, escapedPath)
	var versions []string
	collect := func(body io.Reader) error {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			versions = append(versions, scanner.Text())
		}
		return scanner.Err()
	}
	if err := c.executeRequest(ctx, u, collect); err != nil {
		return nil, err
	}
	return versions, nil
}

// executeRequest executes an HTTP GET request for u, then calls the bodyFunc
// on the response body, if no error occurred.
func (c *Client) executeRequest(ctx context.Context, u string, bodyFunc func(body io.Reader) error) (err error) {
	defer func() {
		if ctx.Err() != nil {
			err = fmt.Errorf("%v: %w", err, derrors.ProxyTimedOut)
		}
		derrors.Wrap(&err, "executeRequest(ctx, %q)", u)
	}()
	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return fmt.Errorf("ctxhttp.Get(ctx, client, %q): %v", u, err)
	}
	defer r.Body.Close()
	switch {
	case 200 <= r.StatusCode && r.StatusCode < 300:
		// OK.
	case r.StatusCode == http.StatusNotFound,
		r.StatusCode == http.StatusGone:
		// Treat both 404 Not Found and 410 Gone responses
		// from the proxy as a "not found" error category.
		// If the response body contains "fetch timed out", treat this
		// as a 504 response so that we retry fetching the module version again
		// later.
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("ioutil.readall: %v", err)
		}
		d := string(data)
		if strings.Contains(d, "fetch timed out") {
			return fmt.Errorf("%q: %w", d, derrors.ProxyTimedOut)
		}
		return fmt.Errorf("%q: %w", d, derrors.NotFound)
	default:
		return fmt.Errorf("unexpected status %d %s", r.StatusCode, r.Status)
	}
	return bodyFunc(r.Body)
}
