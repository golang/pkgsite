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
	"strconv"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/breaker"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/time/rate"
)

// Server receives requests from godoc.org and tees them to pkg.go.dev.
type Server struct {
	limiter *rate.Limiter
	breaker *breaker.Breaker
}

// Config contains configuration values for Server.
type Config struct {
	// Rate is the rate at which requests are rate limited.
	Rate float64
	// Burst is the maximum burst of requests permitted.
	Burst         int
	BreakerConfig breaker.Config
}

// RequestEvent stores information about a godoc.org or pkg.go.dev request.
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

// statusRedBreaker is a custom HTTP status code that denotes that a request
// cannot be handled because the circuit breaker is in the red state.
const statusRedBreaker = 530

var (
	// keyTeeproxyStatus is a census tag for teeproxy response status codes.
	keyTeeproxyStatus = tag.MustNewKey("teeproxy.status")
	// teeproxyGddoLatency holds observed latency in individual teeproxy
	// requests from godoc.org.
	teeproxyGddoLatency = stats.Float64(
		"go-discovery/teeproxy/gddo-latency",
		"Latency of a teeproxy request from godoc.org.",
		stats.UnitMilliseconds,
	)
	// teeproxyPkgGoDevLatency holds observed latency in individual teeproxy
	// requests to pkg.go.dev.
	teeproxyPkgGoDevLatency = stats.Float64(
		"go-discovery/teeproxy/pkgGoDev-latency",
		"Latency of a teeproxy request to pkg.go.dev.",
		stats.UnitMilliseconds,
	)

	// TeeproxyGddoRequestLatencyDistribution aggregates the latency of
	// teeproxy requests from godoc.org by status code.
	TeeproxyGddoRequestLatencyDistribution = &view.View{
		Name:        "go-discovery/teeproxy/gddo-latency",
		Measure:     teeproxyGddoLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Teeproxy latency from godoc.org, by response status code",
		TagKeys:     []tag.Key{keyTeeproxyStatus},
	}
	// TeeproxyPkgGoDevRequestLatencyDistribution aggregates the latency of
	// teeproxy requests to pkg.go.dev by status code.
	TeeproxyPkgGoDevRequestLatencyDistribution = &view.View{
		Name:        "go-discovery/teeproxy/pkgGoDev-latency",
		Measure:     teeproxyPkgGoDevLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Teeproxy latency to pkg.go.dev, by response status code",
		TagKeys:     []tag.Key{keyTeeproxyStatus},
	}
	// TeeproxyGddoRequestCount counts teeproxy requests from godoc.org.
	TeeproxyGddoRequestCount = &view.View{
		Name:        "go-discovery/teeproxy/gddo-count",
		Measure:     teeproxyGddoLatency,
		Aggregation: view.Count(),
		Description: "Count of teeproxy requests from godoc.org",
		TagKeys:     []tag.Key{keyTeeproxyStatus},
	}
	// TeeproxyPkgGoDevRequestCount counts teeproxy requests to pkg.go.dev.
	TeeproxyPkgGoDevRequestCount = &view.View{
		Name:        "go-discovery/teeproxy/pkgGoDev-count",
		Measure:     teeproxyPkgGoDevLatency,
		Aggregation: view.Count(),
		Description: "Count of teeproxy requests to pkg.go.dev",
		TagKeys:     []tag.Key{keyTeeproxyStatus},
	}
)

// NewServer returns a new Server struct with preconfigured settings.
//
// The server is rate limited and allows events up to a rate of "Rate" and
// a burst of "Burst".
//
// The server also implements the circuit breaker pattern and can be in one of
// three states: green, yellow, or red.
//
// In the green state, the server remains green until it encounters an time
// window of length "GreenInterval" where there are more than of "FailsToRed"
// failures and a failureRatio of more than "FailureThreshold", in which case
// the state becomes red.
//
// In the red state, the server halts all requests and waits for a timeout
// period before shifting to the yellow state.
//
// In the yellow state, the server allows the first "SuccsToGreen" requests.
// If any of these fail, the state reverts to red.
// Otherwise, the state becomes green again.
//
// The timeout period is initially set to "MinTimeout" when the breaker shifts
// from green to yellow. By default, the timeout period is doubled each time
// the breaker fails to shift from the yellow state to the green state and is
// capped at "MaxTimeout".
func NewServer(config Config) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer")
	b, err := breaker.New(config.BreakerConfig)
	if err != nil {
		return nil, err
	}
	return &Server{
		limiter: rate.NewLimiter(rate.Limit(config.Rate), config.Burst),
		breaker: b,
	}, nil
}

// ServeHTTP receives requests from godoc.org and forwards them to pkg.go.dev.
// These requests are validated and rate limited before being forwarded. Too
// many error responses returned by pkg.go.dev will cause the server to back
// off temporarily before trying to forward requests to pkg.go.dev again.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, err := s.doRequest(r); err != nil {
		log.Infof(r.Context(), "teeproxy.Server.ServeHTTP: %v", err)
		http.Error(w, http.StatusText(status), status)
		return
	}
}

func (s *Server) doRequest(r *http.Request) (status int, err error) {
	defer derrors.Wrap(&err, "doRequest(%q): referer=%q", r.URL.Path, r.Referer())
	ctx := r.Context()
	if status, err = validateTeeProxyRequest(r); err != nil {
		return status, err
	}
	gddoEvent, err := getGddoEvent(r)
	if err != nil {
		return http.StatusBadRequest, err
	}

	var pkgGoDevEvent *RequestEvent
	defer func() {
		log.Info(ctx, map[string]interface{}{
			"godoc.org":  gddoEvent,
			"pkg.go.dev": pkgGoDevEvent,
			"error":      err,
		})

		var pkgGoDevLatency time.Duration
		if pkgGoDevEvent != nil {
			pkgGoDevLatency = pkgGoDevEvent.Latency
		}
		recordTeeProxyMetric(status, gddoEvent.Latency, pkgGoDevLatency)
	}()
	if experiment.IsActive(r.Context(), internal.ExperimentTeeProxyMakePkgGoDevRequest) {
		if gddoEvent.RedirectHost == "" {
			return http.StatusBadRequest, fmt.Errorf("redirectHost cannot be empty")
		}

		if !s.limiter.Allow() {
			return http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded")
		}

		if !s.breaker.Allow() {
			return statusRedBreaker, fmt.Errorf("breaker is red")
		}
		pkgGoDevEvent, err = makePkgGoDevRequest(ctx, gddoEvent.RedirectHost, pkgGoDevPath(gddoEvent.Path))
		if err != nil {
			return http.StatusInternalServerError, err
		}
		success := pkgGoDevEvent.Status < http.StatusInternalServerError
		s.breaker.Record(success)
		if !success {
			// Use StatusBadGateway to indicate the upstream error.
			return http.StatusBadGateway, fmt.Errorf("%d server error", pkgGoDevEvent.Status)
		}
	}
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

// recordTeeProxyMetric records the latencies and counts of requests from
// godoc.org and to pkg.go.dev, tagged with the response status code.
func recordTeeProxyMetric(status int, gddoLatency, pkgGoDevLatency time.Duration) {
	gddoL := gddoLatency.Seconds() / 1000
	pkgGoDevL := pkgGoDevLatency.Seconds() / 1000

	stats.RecordWithTags(context.Background(), []tag.Mutator{
		tag.Upsert(keyTeeproxyStatus, strconv.Itoa(status)),
	},
		teeproxyGddoLatency.M(gddoL),
		teeproxyPkgGoDevLatency.M(pkgGoDevL),
	)
}
