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

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
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

// despDevClient is the HTTP client used to make requests to deps.dev.
var depsDevClient = &http.Client{Transport: &ochttp.Transport{}}

// depsDevURLGenerator returns a function that will return a URL for the given
// module version on deps.dev. If the URL can't be generated within
// depsDevTimeout then the empty string is returned instead.
func depsDevURLGenerator(ctx context.Context, um *internal.UnitMeta) func() string {
	ctx, cancel := context.WithTimeout(ctx, depsDevTimeout)
	url := make(chan string, 1)
	go func() {
		u, err := fetchDepsDevURL(ctx, um.ModulePath, um.Version)
		switch {
		case errors.Is(err, context.Canceled):
			log.Warningf(ctx, "fetching url from deps.dev: %v", err)
			recordDepsDevMetric(ctx, "canceled")
		case errors.Is(err, context.DeadlineExceeded):
			log.Warningf(ctx, "fetching url from deps.dev: %v", err)
			recordDepsDevMetric(ctx, "timeout")
		case err != nil:
			log.Errorf(ctx, "fetching url from deps.dev: %v", err)
			recordDepsDevMetric(ctx, "error")
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
func fetchDepsDevURL(ctx context.Context, modulePath, version string) (string, error) {
	u := depsDevBase + "/_/s/go" +
		"/p/" + url.PathEscape(modulePath) +
		"/v/" + url.PathEscape(version) +
		"/exists"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	resp, err := depsDevClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", nil // No link to return.
	default:
		return "", errors.New(resp.Status)
	case http.StatusOK:
		// Handled below.
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

var (
	keyDepsDevResult = tag.MustNewKey("depsdev.result")

	depsDevResults = stats.Int64(
		"go-discovery/depsdev_result_count",
		"The result of fetching a deps.dev URL.",
		stats.UnitDimensionless,
	)

	// DepsDevResultCount is a counter of results from fetching a URL from
	// deps.dev: either OK, canceled, timeout, or error.
	DepsDevResultCount = &view.View{
		Name:        "go-discovery/depsdev/result_count",
		Measure:     depsDevResults,
		Aggregation: view.Count(),
		Description: "deps.dev URL fetch results",
		TagKeys:     []tag.Key{keyDepsDevResult},
	}
)

func recordDepsDevMetric(ctx context.Context, kind string) {
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyDepsDevResult, kind),
	}, depsDevResults.M(1))
}
