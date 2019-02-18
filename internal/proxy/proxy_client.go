// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// A Client is used by the fetch service to communicate with a module
// proxy. It handles all methods defined by go help goproxy.
// TODO(julieqiu): Implement GetList, GetMod, and GetZip.
type Client struct {
	url string // URL of the module proxy web server
}

// A VersionInfo contains metadata about a given version of a module.
type VersionInfo struct {
	Version string
	Time    time.Time
}

// cleanURL trims the rawurl of trailing slashes.
func cleanURL(rawurl string) string {
	return strings.TrimRight(rawurl, "/")
}

// New constructs a *Client using the provided rawurl, which is expected to
// be an absolute URI that can be directly passed to http.Get.
func New(rawurl string) *Client {
	return &Client{url: cleanURL(rawurl)}
}

// infoURL constructs a url for a request to
// $GOPROXY/<module>/@v/list.
func (c *Client) infoURL(name, version string) string {
	return fmt.Sprintf("%s/%s/@v/%s.info", c.url, name, version)
}

// GetInfo makes a request to $GOPROXY/<module>/@v/<version>.info and
// transforms that data into a *VersionInfo.
func (c *Client) GetInfo(name, version string) (*VersionInfo, error) {
	r, err := http.Get(c.infoURL(name, version))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var v VersionInfo
	if err = json.NewDecoder(r.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

// zipURL constructs a url for a request to $GOPROXY/<module>/@v/<version>.zip.
func (c *Client) zipURL(name, version string) string {
	return fmt.Sprintf("%s/%s/@v/%s.zip", c.url, name, version)
}

// GetZip makes a request to $GOPROXY/<module>/@v/<version>.zip and transforms
// that data into a *zip.Reader.
func (c *Client) GetZip(name, version string) (*zip.Reader, error) {
	u := c.zipURL(name, version)
	r, err := http.Get(u)
	if err != nil {
		return nil, fmt.Errorf("http.Get(%q): %v",
			c.zipURL(name, version), err)
	}
	defer r.Body.Close()

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, fmt.Errorf("http.Get(%q) returned response: %d (%q)",
			c.zipURL(name, version), r.StatusCode, r.Status)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("http.Get(%q): %v",
			c.zipURL(name, version), err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("http.Get(%q): %v",
			c.zipURL(name, version), err)
	}
	return zipReader, nil
}
