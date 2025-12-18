// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
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

	codeWikiPrefix = "/codewiki?url="
)

var (
	codeWikiURLBase   = "https://codewiki.google/"
	codeWikiExistsURL = "https://codewiki.google/_/exists/"
	codeWikiTimeout   = 1 * time.Second
)

type fetcher func(context.Context, *http.Client) (string, error)

// newURLGenerator returns a function that will return a URL.
// If the URL can't be generated within the timeout then the empty string is returned.
func newURLGenerator(ctx context.Context, client *http.Client, serviceName string, timeout time.Duration, fetch fetcher) func() string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	url := make(chan string, 1)
	go func() {
		u, err := fetch(ctx, client)
		switch {
		case errors.Is(err, context.Canceled):
			log.Warningf(ctx, "fetching url from %s: %v", serviceName, err)
		case errors.Is(err, context.DeadlineExceeded):
			log.Warningf(ctx, "fetching url from %s: %v", serviceName, err)
		case err != nil:
			log.Errorf(ctx, "fetching url from %s: %v", serviceName, err)
		}
		url <- u
	}()
	return func() string {
		defer cancel()
		return <-url
	}
}

// depsDevURLGenerator returns a function that will return a URL for the given
// module version on deps.dev. If the URL can't be generated within
// depsDevTimeout then the empty string is returned instead.
func depsDevURLGenerator(ctx context.Context, client *http.Client, um *internal.UnitMeta) func() string {
	fetch := func(ctx context.Context, client *http.Client) (string, error) {
		return fetchDepsDevURL(ctx, client, um.ModulePath, um.Version)
	}
	return newURLGenerator(ctx, client, "deps.dev", depsDevTimeout, fetch)
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

// codeWikiURLGenerator returns a function that will return a URL for the given
// module version on codewiki. If the URL can't be generated within
// codeWikiTimeout then the empty string is returned instead.
func codeWikiURLGenerator(ctx context.Context, client *http.Client, um *internal.UnitMeta, recordClick bool) func() string {
	fetch := func(ctx context.Context, client *http.Client) (string, error) {
		return fetchCodeWikiURL(ctx, client, um.ModulePath, recordClick)
	}
	return newURLGenerator(ctx, client, "codewiki.google", codeWikiTimeout, fetch)
}

// fetchCodeWikiURL makes a request to codewiki to check whether the given
// path is known there, and if so it returns the link to that page.
func fetchCodeWikiURL(ctx context.Context, client *http.Client, path string, recordClick bool) (string, error) {
	if strings.HasPrefix(path, "golang.org/x/") {
		path = strings.Replace(path, "golang.org/x/", "github.com/golang/", 1)
	}
	// TODO: Add support for other hosts as needed.
	if !strings.HasPrefix(path, "github.com/") {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", codeWikiExistsURL+path, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(resp.Status)
	}
	res := codeWikiURLBase + path
	if recordClick {
		res = codeWikiPrefix + res
	}
	return res, nil
}
