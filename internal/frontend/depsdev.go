// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
)

const (
	// depsDevBase is the base URL for requests to deps.dev.
	// It should not include a trailing slash.
	depsDevBase = "https://deps.dev"
	// depsDevTimeout is the time budget for making requests to deps.dev.
	depsDevTimeout = 250 * time.Millisecond
)

// depsDevURLGenerator returns a function that will return a URL for the given
// module version on deps.dev. If the URL can't be generated within
// depsDevTimeout then the empty string is returned instead.
func depsDevURLGenerator(ctx context.Context, client *http.Client, um *internal.UnitMeta) func() string {
	ctx, cancel := context.WithTimeout(ctx, depsDevTimeout)
	url := make(chan string, 1)
	go func() {
		u, err := fetchDepsDevURL(ctx, client, um.ModulePath, um.Version)
		switch {
		case errors.Is(err, context.Canceled):
			log.Warningf(ctx, "fetching url from deps.dev: %v", err)
		case errors.Is(err, context.DeadlineExceeded):
			log.Warningf(ctx, "fetching url from deps.dev: %v", err)
		case err != nil:
			log.Errorf(ctx, "fetching url from deps.dev: %v", err)
		}
		url <- u
	}()
	return func() string {
		defer cancel()
		return <-url
	}
}

// fetchDepsDevURL makes a request to deps.dev to check whether the given
// module version is known there, and if so it returns the link to that module
// version page on deps.dev.
func fetchDepsDevURL(ctx context.Context, client *http.Client, modulePath, version string) (string, error) {
	u := depsDevBase + "/_/s/go" +
		"/p/" + url.PathEscape(modulePath) +
		"/v/" + url.PathEscape(version) +
		"/exists"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", nil // No link to return.
	case http.StatusOK:
		// Handled below.
	default:
		return "", errors.New(resp.Status)
	}
	var r struct {
		stem, Name, Version string
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.Name == "" || r.Version == "" {
		return "", errors.New("name or version unset in response")
	}
	return depsDevBase + "/go/" + url.PathEscape(r.Name) + "/" + url.PathEscape(r.Version), nil
}
