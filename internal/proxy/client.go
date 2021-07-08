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
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// A Client is used by the fetch service to communicate with a module
// proxy. It handles all methods defined by go help goproxy.
type Client struct {
	// URL of the module proxy web server
	url string

	// Client used for HTTP requests. It is mutable for testing purposes.
	httpClient *http.Client

	// Whether fetch should be disabled.
	disableFetch bool

	// One-element zip cache, to avoid a double download.
	// See TestFetchAndUpdateStateCacheZip in internal/worker/fetch_test.go.
	// Not thread-safe; should be used by only a single request goroutine.
	rememberLastZip                   bool
	lastZipModulePath, lastZipVersion string
	lastZipReader                     *zip.Reader
}

// A VersionInfo contains metadata about a given version of a module.
type VersionInfo struct {
	Version string
	Time    time.Time
}

// Setting this header to true prevents the proxy from fetching uncached
// modules.
const disableFetchHeader = "Disable-Module-Fetch"

// New constructs a *Client using the provided url, which is expected to
// be an absolute URI that can be directly passed to http.Get.
func New(u string) (_ *Client, err error) {
	defer derrors.WrapStack(&err, "proxy.New(%q)", u)
	return &Client{
		url:          strings.TrimRight(u, "/"),
		httpClient:   &http.Client{Transport: &ochttp.Transport{}},
		disableFetch: false,
	}, nil
}

// WithFetchDisabled returns a new client that sets the Disable-Module-Fetch
// header so that the proxy does not fetch a module it doesn't already know
// about.
func (c *Client) WithFetchDisabled() *Client {
	c2 := *c
	c2.disableFetch = true
	return &c2
}

// FetchDisabled reports whether proxy fetch is disabled.
func (c *Client) FetchDisabled() bool {
	return c.disableFetch
}

// WithZipCache returns a new client that caches the last zip
// it downloads (not thread-safely).
func (c *Client) WithZipCache() *Client {
	c2 := *c
	c2.rememberLastZip = true
	c2.lastZipModulePath = ""
	c2.lastZipVersion = ""
	c2.lastZipReader = nil
	return &c2
}

// Info makes a request to $GOPROXY/<module>/@v/<requestedVersion>.info and
// transforms that data into a *VersionInfo.
// If requestedVersion is internal.LatestVersion, it uses the proxy's @latest
// endpoint instead.
func (c *Client) Info(ctx context.Context, modulePath, requestedVersion string) (_ *VersionInfo, err error) {
	defer func() {
		// Don't report NotFetched, because it is the normal result of fetching
		// an uncached module when fetch is disabled.
		// Don't report timeouts, because they are relatively frequent and not actionable.
		wrap := derrors.Wrap
		if !errors.Is(err, derrors.NotFetched) && !errors.Is(err, derrors.ProxyTimedOut) {
			wrap = derrors.WrapAndReport
		}
		wrap(&err, "proxy.Client.Info(%q, %q)", modulePath, requestedVersion)
	}()
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

// Mod makes a request to $GOPROXY/<module>/@v/<resolvedVersion>.mod and returns the raw data.
func (c *Client) Mod(ctx context.Context, modulePath, resolvedVersion string) (_ []byte, err error) {
	defer derrors.WrapStack(&err, "proxy.Client.Mod(%q, %q)", modulePath, resolvedVersion)
	return c.readBody(ctx, modulePath, resolvedVersion, "mod")
}

// Zip makes a request to $GOPROXY/<modulePath>/@v/<resolvedVersion>.zip and
// transforms that data into a *zip.Reader. <resolvedVersion> must have already
// been resolved by first making a request to
// $GOPROXY/<modulePath>/@v/<requestedVersion>.info to obtained the valid
// semantic version.
func (c *Client) Zip(ctx context.Context, modulePath, resolvedVersion string) (_ *zip.Reader, err error) {
	defer derrors.WrapStack(&err, "proxy.Client.Zip(ctx, %q, %q)", modulePath, resolvedVersion)

	if c.lastZipModulePath == modulePath && c.lastZipVersion == resolvedVersion {
		return c.lastZipReader, nil
	}
	bodyBytes, err := c.readBody(ctx, modulePath, resolvedVersion, "zip")
	if err != nil {
		return nil, err
	}
	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("zip.NewReader: %v: %w", err, derrors.BadModule)
	}
	if c.rememberLastZip {
		c.lastZipModulePath = modulePath
		c.lastZipVersion = resolvedVersion
		c.lastZipReader = zipReader
	}
	return zipReader, nil
}

// ZipSize gets the size in bytes of the zip from the proxy, without downloading it.
// The version must be resolved, as by a call to Client.Info.
func (c *Client) ZipSize(ctx context.Context, modulePath, resolvedVersion string) (_ int64, err error) {
	defer derrors.WrapStack(&err, "proxy.Client.ZipSize(ctx, %q, %q)", modulePath, resolvedVersion)

	url, err := c.escapedURL(modulePath, resolvedVersion, "zip")
	if err != nil {
		return 0, err
	}
	res, err := ctxhttp.Head(ctx, c.httpClient, url)
	if err != nil {
		return 0, fmt.Errorf("ctxhttp.Head(ctx, client, %q): %v", url, err)
	}
	defer res.Body.Close()
	if err := responseError(res, false); err != nil {
		return 0, err
	}
	if res.ContentLength < 0 {
		return 0, errors.New("unknown content length")
	}
	return res.ContentLength, nil
}

func (c *Client) escapedURL(modulePath, requestedVersion, suffix string) (_ string, err error) {
	defer derrors.WrapStack(&err, "Client.escapedURL(%q, %q, %q)", modulePath, requestedVersion, suffix)

	if suffix != "info" && suffix != "mod" && suffix != "zip" {
		return "", errors.New(`suffix must be "info", "mod" or "zip"`)
	}
	escapedPath, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("path: %v: %w", err, derrors.InvalidArgument)
	}
	if requestedVersion == version.Latest {
		if suffix != "info" {
			return "", fmt.Errorf("cannot ask for latest with suffix %q", suffix)
		}
		return fmt.Sprintf("%s/%s/@latest", c.url, escapedPath), nil
	}
	escapedVersion, err := module.EscapeVersion(requestedVersion)
	if err != nil {
		return "", fmt.Errorf("version: %v: %w", err, derrors.InvalidArgument)
	}
	return fmt.Sprintf("%s/%s/@v/%s.%s", c.url, escapedPath, escapedVersion, suffix), nil
}

func (c *Client) readBody(ctx context.Context, modulePath, requestedVersion, suffix string) (_ []byte, err error) {
	defer derrors.WrapStack(&err, "Client.readBody(%q, %q, %q)", modulePath, requestedVersion, suffix)

	u, err := c.escapedURL(modulePath, requestedVersion, suffix)
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

// Versions makes a request to $GOPROXY/<path>/@v/list and returns the
// resulting version strings.
func (c *Client) Versions(ctx context.Context, modulePath string) (_ []string, err error) {
	defer derrors.Wrap(&err, "Versions(ctx, %q)", modulePath)
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
		derrors.WrapStack(&err, "executeRequest(ctx, %q)", u)
	}()

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	if c.disableFetch {
		req.Header.Set(disableFetchHeader, "true")
	}
	r, err := ctxhttp.Do(ctx, c.httpClient, req)
	if err != nil {
		return fmt.Errorf("ctxhttp.Do(ctx, client, %q): %v", u, err)
	}
	defer r.Body.Close()
	if err := responseError(r, c.disableFetch); err != nil {
		return err
	}
	return bodyFunc(r.Body)
}

// responseError translates the response status code to an appropriate error.
func responseError(r *http.Response, fetchDisabled bool) error {
	switch {
	case 200 <= r.StatusCode && r.StatusCode < 300:
		return nil
	case 500 <= r.StatusCode:
		return derrors.ProxyError
	case r.StatusCode == http.StatusNotFound,
		r.StatusCode == http.StatusGone:
		// Treat both 404 Not Found and 410 Gone responses
		// from the proxy as a "not found" error category.
		// If the response body contains "fetch timed out", treat this
		// as a 504 response so that we retry fetching the module version again
		// later.
		//
		// If the Disable-Module-Fetch header was set, use a different
		// error code so we can tell the difference.
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("ioutil.readall: %v", err)
		}
		d := string(data)
		switch {
		case strings.Contains(d, "fetch timed out"):
			err = derrors.ProxyTimedOut
		case fetchDisabled:
			err = derrors.NotFetched
		default:
			err = derrors.NotFound
		}
		return fmt.Errorf("%q: %w", d, err)
	default:
		return fmt.Errorf("unexpected status %d %s", r.StatusCode, r.Status)
	}
}
