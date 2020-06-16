// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package teeproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
)

type RequestEvent struct {
	Host    string
	Path    string
	URL     string
	Header  http.Header
	Latency time.Duration
	Status  int
	Error   string

	// RedirectHost indicates where a request should be redirected to. It is
	// used for testing when redirecting requests to somewhere other than
	// pkg.go.dev.
	RedirectHost string
	// IsRobot reports whether this request came from a robot.
	// https://github.com/golang/gddo/blob/a4ebd2f/gddo-server/main.go#L152
	IsRobot bool
}

var gddoToPkgGoDevRequest = map[string]string{
	"/":                              "/",
	"/-/about":                       "/about",
	"/-/bootstrap.min.css":           "/404",
	"/-/bootstrap.min.js":            "/404",
	"/-/bot":                         "/404",
	"/-/go":                          "/std",
	"/-/jquery-2.0.3.min.js":         "/404",
	"/-/refresh":                     "/404",
	"/-/sidebar.css":                 "/404",
	"/-/site.css":                    "/404",
	"/-/subrepo":                     "/404",
	"/BingSiteAuth.xml":              "/404",
	"/C":                             "/C",
	"/favicon.ico":                   "/favicon.ico",
	"/google3d2f3cd4cc2bb44b.html":   "/404",
	"/humans.txt":                    "/404",
	"/robots.txt":                    "/404",
	"/site.js":                       "/404",
	"/third_party/jquery.timeago.js": "/404",
}

func HandleGddoEvent(w http.ResponseWriter, r *http.Request) {
	if status, err := doRequest(r); err != nil {
		log.Infof(r.Context(), "teeproxy.HandleGddoEvent: %v", err)
		http.Error(w, http.StatusText(status), status)
		return
	}
}

func doRequest(r *http.Request) (_ int, err error) {
	defer derrors.Wrap(&err, "doRequest(%q): referer=%q", r.URL.Path, r.Referer())
	ctx := r.Context()
	status, err := validateTeeProxyRequest(r)
	if err != nil {
		return status, err
	}
	gddoEvent, err := getGddoEvent(r)
	if err != nil {
		return http.StatusBadRequest, err
	}

	var pkgGoDevEvent *RequestEvent
	if experiment.IsActive(r.Context(), internal.ExperimentTeeProxyMakePkgGoDevRequest) {
		pkgGoDevEvent, err = makePkgGoDevRequest(ctx, gddoEvent.RedirectHost, pkgGoDevPath(gddoEvent.Path))
		if err != nil {
			log.Info(ctx, map[string]*RequestEvent{
				"godoc.org": gddoEvent,
			})
			return http.StatusInternalServerError, err
		}
	}
	log.Info(ctx, map[string]*RequestEvent{
		"godoc.org":  gddoEvent,
		"pkg.go.dev": pkgGoDevEvent,
	})
	return http.StatusOK, nil
}

// validateTeeProxyRequest validates that a request to the teeproxy is allowed.
// It will return the error code and error if a request is invalid. Otherwise,
// it will return http.StatusOK.
func validateTeeProxyRequest(r *http.Request) (code int, err error) {
	defer derrors.Wrap(&err, "validateTeeProxyRequest(r)")
	if r.Method != "POST" {
		return http.StatusMethodNotAllowed, fmt.Errorf("%s: %q", http.StatusText(http.StatusMethodNotAllowed), r.Method)
	}
	ct := r.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		return http.StatusUnsupportedMediaType, fmt.Errorf("Content-Type %q is not supported", ct)
	}
	return http.StatusOK, nil
}

// pkgGoDevPath returns the corresponding path on pkg.go.dev for the given
// godoc.org path.
func pkgGoDevPath(gddoPath string) string {
	redirectPath, ok := gddoToPkgGoDevRequest[gddoPath]
	if ok {
		return redirectPath
	}
	return gddoPath
}

// getGddoEvent constructs a url.URL and RequestEvent from the request.
func getGddoEvent(r *http.Request) (gddoEvent *RequestEvent, err error) {
	defer func() {
		derrors.Wrap(&err, "getGddoEvent(r)")
		if gddoEvent != nil && err != nil {
			log.Info(r.Context(), map[string]interface{}{
				"godoc.org": gddoEvent,
				"tee-error": err.Error(),
			})
		}
	}()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	gddoEvent = &RequestEvent{}
	if err := json.Unmarshal(body, gddoEvent); err != nil {
		return nil, err
	}
	return gddoEvent, nil
}

// makePkgGoDevRequest makes a request to the redirectHost and redirectPath,
// and returns a requestEvent based on the output.
func makePkgGoDevRequest(ctx context.Context, redirectHost, redirectPath string) (_ *RequestEvent, err error) {
	defer derrors.Wrap(&err, "makePkgGoDevRequest(%q, %q)", redirectHost, redirectPath)
	if redirectHost == "" {
		return nil, fmt.Errorf("redirectHost cannot be empty")
	}
	redirectURL := redirectHost + redirectPath
	req, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := ctxhttp.Do(ctx, http.DefaultClient, req)
	if err != nil {
		return nil, err
	}
	return &RequestEvent{
		Host:    redirectHost,
		Path:    redirectPath,
		URL:     redirectURL,
		Status:  resp.StatusCode,
		Latency: time.Since(start),
	}, nil
}
