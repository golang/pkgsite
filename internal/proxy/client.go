// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/dzip"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/net/context/ctxhttp"
)

const (
	stdlibModulePathProxy = "go.googlesource.com/go.git"
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

// New constructs a *Client using the provided rawurl, which is expected to
// be an absolute URI that can be directly passed to http.Get.
func New(rawurl string) (*Client, error) {
	url, err := url.Parse(rawurl)
	if err != nil {
		return nil, fmt.Errorf("url.Parse(%q): %v", rawurl, err)
	}
	if url.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be https (got %s)", url.Scheme)
	}
	return &Client{url: cleanURL(rawurl), httpClient: http.DefaultClient}, nil
}

// cleanURL trims the rawurl of trailing slashes.
func cleanURL(rawurl string) string {
	return strings.TrimRight(rawurl, "/")
}

// GetInfo makes a request to $GOPROXY/<module>/@v/<version>.info and
// transforms that data into a *VersionInfo.
func (c *Client) GetInfo(ctx context.Context, path, version string) (*VersionInfo, error) {
	v, err := c.getInfo(ctx, path, version)
	if err != nil {
		return nil, err
	}
	if path == "std" {
		v.Version = semver.Canonical(version)
	}
	return v, nil
}

func (c *Client) getInfo(ctx context.Context, path, version string) (*VersionInfo, error) {
	u, err := c.infoURL(path, version)
	if err != nil {
		return nil, fmt.Errorf("infoURL(%q, %q): %v", path, version, err)
	}

	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, client, %q): %v", u, err)
	}
	defer r.Body.Close()

	if err := derrors.StatusError(r.StatusCode, "ctxhttp.Get(ctx, client, %q)", u); err != nil {
		return nil, err
	}

	var v VersionInfo
	if err = json.NewDecoder(r.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

// infoURL constructs a url for a request to $GOPROXY/<module>/@v/<version>.info.
// If the path is "std", the version will be converted to the corresponding go
// tag. For example, "v1.12.5" will be converted to "go1.12.5" and "v1.12.0"
// will be converted to "go1.12".
func (c *Client) infoURL(path, version string) (string, error) {
	if path == "std" {
		path = stdlibModulePathProxy
		version = fmt.Sprintf("go%s", strings.TrimSuffix(strings.TrimPrefix(version, "v"), ".0"))
	}
	path, version, err := encodeModulePathAndVersion(path, version)
	if err != nil {
		return "", fmt.Errorf("encodeModulePathAndVersion(%q, %q): %v", path, version, err)
	}
	return fmt.Sprintf("%s/%s/@v/%s.info", c.url, path, version), nil
}

// GetZip makes a request to $GOPROXY/<path>/@v/<version>.zip and transforms
// that data into a *zip.Reader. <version> is obtained by first making a
// request to $GOPROXY/<path>/@v/<requestedVersion>.info to obtained the valid
// semantic version.
func (c *Client) GetZip(ctx context.Context, path, requestedVersion string) (*zip.Reader, error) {
	info, err := c.getInfo(ctx, path, requestedVersion)
	if err != nil {
		return nil, err
	}
	zipReader, err := c.getZip(ctx, path, info.Version)
	if err != nil {
		return nil, err
	}
	if path == "std" {
		return createGoZipReader(zipReader, info.Version, requestedVersion)
	}
	return zipReader, nil
}

// getZip makes a request to $GOPROXY/<path>/@v/<version>.zip and transforms
// that data into a *zip.Reader.
func (c *Client) getZip(ctx context.Context, path, version string) (*zip.Reader, error) {
	u, err := c.zipURL(path, version)
	if err != nil {
		return nil, fmt.Errorf("zipURL(%q, %q): %v", path, version, err)
	}

	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, nil, %q): %v", u, err)
	}
	defer r.Body.Close()

	if err := derrors.StatusError(r.StatusCode, "HTTP error from proxy for %q: %d", u, r.StatusCode); err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("get(ctx, %q): %v", u, err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("get(ctx, %q): %v", u, err)
	}
	return zipReader, nil
}

// zipURL constructs a url for a request to $GOPROXY/<module>/@v/<version>.zip.
func (c *Client) zipURL(path, version string) (string, error) {
	if path == "std" {
		path = stdlibModulePathProxy
	}
	path, version, err := encodeModulePathAndVersion(path, version)
	if err != nil {
		return "", fmt.Errorf("encodeModulePathAndVersion(%q, %q): %v", path, version, err)
	}
	return fmt.Sprintf("%s/%s/@v/%s.zip", c.url, path, version), nil
}

// createGoZipReader returns a *zip.Reader containing the README, LICENSE and
// contents of the src/ directory for a zip obtained from a request to
// $GOPROXY/go.googlesource.com/go.git/@v/<version>.zip. The filenames returned
// will be trimmed of the prefix go.googlesource.com@<infoVersion> or
// go.googlesource.com@<infoVersion>/src. The prefix std@<goVersion> will be
// added to each of the resulting filenames.
func createGoZipReader(r *zip.Reader, infoVersion string, goVersion string) (*zip.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	for _, file := range r.File {
		stdlibFilePrefix := fmt.Sprintf("%s@%s/", stdlibModulePathProxy, infoVersion)
		if !strings.HasPrefix(file.Name, stdlibFilePrefix+"src/") &&
			!strings.HasPrefix(file.Name, stdlibFilePrefix+"README") &&
			!strings.HasPrefix(file.Name, stdlibFilePrefix+"LICENSE") {
			continue
		}

		// Trim stdlibFilePrefix from README and LICENSE, and
		// stdlibFilePrefix+"src/" from files in the src/ directory.
		fileName := fmt.Sprintf("std@%s/%s", goVersion, strings.TrimPrefix(strings.TrimPrefix(file.Name, stdlibFilePrefix), "src/"))
		f, err := w.Create(fileName)
		if err != nil {
			return nil, fmt.Errorf("w.Create(%q): %v", file.Name, err)
		}

		contents, err := dzip.ReadZipFile(file)
		if err != nil {
			log.Printf("zip.ReadZipFile(%q): %v", file.Name, err)
			continue
		}

		if _, err = f.Write(contents); err != nil {
			return nil, fmt.Errorf("f.Write: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("w.Close(): %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return nil, fmt.Errorf("zip.NewReader: %v", err)
	}
	return zipReader, nil
}

func encodeModulePathAndVersion(path, version string) (string, string, error) {
	encodedPath, err := module.EncodePath(path)
	if err != nil {
		return "", "", fmt.Errorf("module.EncodePath(%q): %v", path, err)
	}
	encodedVersion, err := module.EncodeVersion(version)
	if err != nil {
		return "", "", fmt.Errorf("module.EncodeVersion(%q): %v", version, err)
	}
	return encodedPath, encodedVersion, nil
}
