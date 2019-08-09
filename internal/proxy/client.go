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

	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/dzip"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/xerrors"
)

const (
	stdlibProxyModulePathPrefix = "go.googlesource.com/go.git"
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
	return &Client{url: cleanURL(rawurl), httpClient: &http.Client{Transport: &ochttp.Transport{}}}, nil
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
	if internal.IsStandardLibraryModule(path) {
		v.Version = semver.Canonical(version)
	}
	return v, nil
}

func (c *Client) getInfo(ctx context.Context, requestedPath, requestedVersion string) (*VersionInfo, error) {
	path, version, err := modulePathAndVersionForProxyRequest(requestedPath, requestedVersion)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/%s/@v/%s.info", c.url, path, version)
	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, client, %q): %v", u, err)
	}
	defer r.Body.Close()

	if err := derrors.FromHTTPStatus(r.StatusCode, "ctxhttp.Get(ctx, client, %q)", u); err != nil {
		return nil, err
	}

	var v VersionInfo
	if err = json.NewDecoder(r.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetZip makes a request to $GOPROXY/<path>/@v/<version>.zip and transforms
// that data into a *zip.Reader. <version> is obtained by first making a
// request to $GOPROXY/<path>/@v/<version>.info to obtained the valid
// semantic version.
func (c *Client) GetZip(ctx context.Context, requestedPath, requestedVersion string) (*zip.Reader, error) {
	info, err := c.getInfo(ctx, requestedPath, requestedVersion)
	if err != nil {
		return nil, err
	}
	zipPath, _, err := modulePathAndVersionForProxyRequest(requestedPath, requestedVersion)
	if err != nil {
		return nil, err
	}
	zipReader, err := c.getZip(ctx, zipPath, info.Version)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(zipPath, stdlibProxyModulePathPrefix) {
		return createGoZipReader(zipReader, requestedPath, info.Version, requestedVersion)
	}
	return zipReader, nil
}

// getZip makes a request to $GOPROXY/<proxyModulePath>/@v/<proxyVersion>.zip
// and transforms that data into a *zip.Reader. proxyPath and proxyVersion are
// expected to be encoded for a proxy request. proxyVersion is expected to be a
// valid semantic version.
func (c *Client) getZip(ctx context.Context, proxyModulePath, proxyVersion string) (*zip.Reader, error) {
	u := fmt.Sprintf("%s/%s/@v/%s.zip", c.url, proxyModulePath, proxyVersion)
	r, err := ctxhttp.Get(ctx, c.httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, nil, %q): %v", u, err)
	}
	defer r.Body.Close()

	if err := derrors.FromHTTPStatus(r.StatusCode, "HTTP error from proxy for %q: %d", u, r.StatusCode); err != nil {
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

// createGoZipReader returns a *zip.Reader containing the README, LICENSE and
// contents of the src/ directory for a zip obtained from a request to
// $GOPROXY/go.googlesource.com/go.git/@v/<version>.zip. The filenames returned
// will be trimmed of the prefix go.googlesource.com@<pseudoVersion> or
// go.googlesource.com@<pseudoVersion>/src. The prefix std@<requestedSemanticVersion> will be
// added to each of the resulting filenames.
func createGoZipReader(r *zip.Reader, path, pseudoVersion, requestedSemanticVersion string) (*zip.Reader, error) {
	proxyPath, _, err := modulePathAndVersionForProxyRequest(path, requestedSemanticVersion)
	if err != nil {
		return nil, err
	}

	var originalZipFilePrefix string
	if semver.MajorMinor(requestedSemanticVersion) != "v1.13" {
		originalZipFilePrefix = fmt.Sprintf("%s@%s/src", proxyPath, pseudoVersion)
	} else {
		originalZipFilePrefix = fmt.Sprintf("%s@%s", proxyPath, pseudoVersion)
	}
	newZipFilePrefix := fmt.Sprintf("%s@%s", path, requestedSemanticVersion)

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, file := range r.File {
		preVersion113Root := strings.TrimSuffix(originalZipFilePrefix, "/src")
		if !strings.HasPrefix(
			file.Name, originalZipFilePrefix) &&
			!strings.HasPrefix(file.Name, preVersion113Root+"/README") &&
			!strings.HasPrefix(file.Name, preVersion113Root+"/LICENSE") {
			continue
		}

		var fileName string
		if semver.MajorMinor(requestedSemanticVersion) == "v1.13" {
			fileName = newZipFilePrefix + strings.TrimPrefix(file.Name, originalZipFilePrefix)
		} else {
			// Trim originalZipFilePrefix from README and LICENSE, and
			// originalZipFilePrefix+"src" from files in the src/ directory.
			fileName = newZipFilePrefix + strings.TrimPrefix(strings.TrimPrefix(file.Name, preVersion113Root), "/src")
		}

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

func modulePathAndVersionForProxyRequest(path, version string) (string, string, error) {
	if !internal.IsStandardLibraryModule(path) {
		return encodeModulePathAndVersion(path, version)
	}
	if !semver.IsValid(version) {
		return "", "", derrors.FromHTTPStatus(http.StatusBadRequest, "requests for std must provide a valid semantic version: %q ", version)
	}
	if path == "cmd" {
		if semver.MajorMinor(version) != "v1.13" {
			return "", "", derrors.FromHTTPStatus(http.StatusBadRequest, "module cmd can only be fetched for versions v1.13.x: version = %q", version)
		}
		path = fmt.Sprintf("%s/src/cmd", stdlibProxyModulePathPrefix)
	} else if path == "std" {
		if semver.MajorMinor(version) == "v1.13" {
			path = fmt.Sprintf("%s/src", stdlibProxyModulePathPrefix)
		} else {
			path = stdlibProxyModulePathPrefix
		}
	}
	if strings.HasPrefix(path, stdlibProxyModulePathPrefix) {
		ver, err := internal.GoVersionForSemanticVersion(version)
		if err != nil {
			return "", "", xerrors.Errorf("GoVersionForSemanticVersion(%q): %v: %w", version, err, derrors.InvalidArgument)
		}
		version = ver
	}
	return path, version, nil
}

func encodeModulePathAndVersion(path, version string) (string, string, error) {
	encodedPath, err := module.EncodePath(path)
	if err != nil {
		return "", "", derrors.FromHTTPStatus(http.StatusBadRequest, "module.EncodePath(%q): %v", path, err)
	}
	encodedVersion, err := module.EncodeVersion(version)
	if err != nil {
		return "", "", derrors.FromHTTPStatus(http.StatusBadRequest, "module.EncodeVersion(%q): %v", version, err)
	}
	return encodedPath, encodedVersion, nil
}
