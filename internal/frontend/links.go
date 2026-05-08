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
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
)

const (
	// depsDevBase is the base URL for requests to deps.dev.
	// It should not include a trailing slash.
	depsDevBase = "https://deps.dev"
	// depsDevTimeout is the time budget for making requests to deps.dev.
	depsDevTimeout = 250 * time.Millisecond

	codeWikiPrefix    = "/codewiki?"
	attributionParams = "?utm_source=first_party_link&utm_medium=go_pkg_web&utm_campaign="
)

var (
	codeWikiURLBase   = "https://codewiki.google/"
	codeWikiExistsURL = "https://codewiki.google/_/exists/"
	codeWikiTimeout   = 1 * time.Second
)

// goPrivateConfig holds the GOPRIVATE and GONOPROXY values as reported by
// "go env". ok is false when the values could not be determined (e.g. "go" is
// not on PATH); in that case pkgsite conservatively treats every module as
// private and skips external link lookups.
type goPrivateConfig struct {
	goprivate string
	gonoproxy string
	ok        bool
}

// goPrivatePatterns returns the GOPRIVATE and GONOPROXY values, reading them
// via "go env" so that values set in the user's go env file (via "go env -w")
// are honored in addition to process environment variables.
//
// The values are cached after the first call. Tests may replace this variable.
var goPrivatePatterns = sync.OnceValue(loadGoPrivatePatterns)

func loadGoPrivatePatterns() goPrivateConfig {
	out, err := exec.Command("go", "env", "-json", "GOPRIVATE", "GONOPROXY").Output()
	if err != nil {
		log.Warningf(context.Background(), "reading GOPRIVATE/GONOPROXY via 'go env': %v; external link lookups will be skipped", err)
		return goPrivateConfig{}
	}
	var v struct {
		GOPRIVATE string
		GONOPROXY string
	}
	if err := json.Unmarshal(out, &v); err != nil {
		log.Warningf(context.Background(), "parsing 'go env' output: %v; external link lookups will be skipped", err)
		return goPrivateConfig{}
	}
	return goPrivateConfig{goprivate: v.GOPRIVATE, gonoproxy: v.GONOPROXY, ok: true}
}

// isPrivateModulePath reports whether modulePath matches either of the given
// GOPRIVATE or GONOPROXY pattern lists, indicating that pkgsite should not
// consult external services about it.
func isPrivateModulePath(modulePath, goprivate, gonoproxy string) bool {
	return module.MatchPrefixPatterns(goprivate, modulePath) ||
		module.MatchPrefixPatterns(gonoproxy, modulePath)
}

// externalLinkGenerators returns URL-generator functions for deps.dev and
// codewiki.google for the given unit. If suppressed (because pkgsite is in
// goDocMode, the GOPRIVATE/GONOPROXY values can't be determined, or the module
// path is matched by GOPRIVATE/GONOPROXY), the returned generators return the
// empty string and make no network requests.
func externalLinkGenerators(ctx context.Context, client *http.Client, um *internal.UnitMeta, goDocMode, recordCodeWikiClicks bool) (depsDev, codeWiki func() string) {
	empty := func() string { return "" }
	cfg := goPrivatePatterns()
	if goDocMode || !cfg.ok || isPrivateModulePath(um.ModulePath, cfg.goprivate, cfg.gonoproxy) {
		return empty, empty
	}
	return depsDevURLGenerator(ctx, client, um), codeWikiURLGenerator(ctx, client, um, recordCodeWikiClicks)
}

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
		return fetchCodeWikiURL(ctx, client, um, recordClick)
	}
	return newURLGenerator(ctx, client, "codewiki.google", codeWikiTimeout, fetch)
}

// fetchCodeWikiURL makes a request to codewiki to check whether the given
// path is known there, and if so it returns the link to that page.
func fetchCodeWikiURL(ctx context.Context, client *http.Client, um *internal.UnitMeta, recordClick bool) (string, error) {
	path := um.ModulePath
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
	// Handle 404 as a successful "not found" state rather than an error.
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(resp.Status)
	}
	res := codeWikiURLBase + path + attributionParams + path
	if recordClick {
		v := url.Values{}
		v.Set("url", res)
		v.Set("module", um.ModulePath)
		v.Set("package", um.Path)
		res = codeWikiPrefix + v.Encode()
	}
	return res, nil
}
