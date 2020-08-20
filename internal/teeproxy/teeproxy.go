// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package teeproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal/breaker"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/time/rate"
)

// Server receives requests from godoc.org and tees them to specified hosts.
type Server struct {
	hosts    []string
	client   *http.Client
	limiter  *rate.Limiter
	breakers map[string]*breaker.Breaker
	// authKey and authValue are used to indicate to pkg.go.dev that the
	// request is coming from the teeproxy.
	authKey, authValue string
}

// Config contains configuration values for Server.
type Config struct {
	// AuthKey is the name of the header that is used by pkg.go.dev to
	// determine if a request is coming from a trusted source.
	AuthKey string
	// AuthValue is the value of the header that is used by pkg.go.dev to
	// determine that the request is coming from the teeproxy.
	AuthValue string
	// Hosts is the list of hosts that the teeproxy forwards requests to.
	Hosts []string
	// Client is the HTTP client used by the teeproxy to forward requests
	// to the hosts.
	Client *http.Client
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
	Error   error
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
	// keyTeeproxyHost is a census tag for hosts that teeproxy forward requests to.
	keyTeeproxyHost = tag.MustNewKey("teeproxy.host")
	// keyTeeproxyPath is a census tag for godoc.org paths that don't work in
	// pkg.go.dev.
	keyTeeproxyPath = tag.MustNewKey("teeproxy.path")
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
	// teeproxyPkgGoDevBrokenPaths counts broken paths in pkg.go.dev that work
	// in godoc.org
	teeproxyPkgGoDevBrokenPaths = stats.Int64(
		"go-discovery/teeproxy/pkgGoDev-brokenPaths",
		"Count of paths that error in pkg.go.dev but 200 in godoc.org.",
		stats.UnitDimensionless,
	)

	// TeeproxyGddoRequestLatencyDistribution aggregates the latency of
	// teeproxy requests from godoc.org by status code and host.
	TeeproxyGddoRequestLatencyDistribution = &view.View{
		Name:        "go-discovery/teeproxy/gddo-latency",
		Measure:     teeproxyGddoLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Teeproxy latency from godoc.org, by response status code",
		TagKeys:     []tag.Key{keyTeeproxyStatus, keyTeeproxyHost},
	}
	// TeeproxyPkgGoDevRequestLatencyDistribution aggregates the latency of
	// teeproxy requests to pkg.go.dev by status code and host.
	TeeproxyPkgGoDevRequestLatencyDistribution = &view.View{
		Name:        "go-discovery/teeproxy/pkgGoDev-latency",
		Measure:     teeproxyPkgGoDevLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Teeproxy latency to pkg.go.dev, by response status code",
		TagKeys:     []tag.Key{keyTeeproxyStatus, keyTeeproxyHost},
	}
	// TeeproxyGddoRequestCount counts teeproxy requests from godoc.org.
	TeeproxyGddoRequestCount = &view.View{
		Name:        "go-discovery/teeproxy/gddo-count",
		Measure:     teeproxyGddoLatency,
		Aggregation: view.Count(),
		Description: "Count of teeproxy requests from godoc.org",
		TagKeys:     []tag.Key{keyTeeproxyStatus, keyTeeproxyHost},
	}
	// TeeproxyPkgGoDevRequestCount counts teeproxy requests to pkg.go.dev.
	TeeproxyPkgGoDevRequestCount = &view.View{
		Name:        "go-discovery/teeproxy/pkgGoDev-count",
		Measure:     teeproxyPkgGoDevLatency,
		Aggregation: view.Count(),
		Description: "Count of teeproxy requests to pkg.go.dev",
		TagKeys:     []tag.Key{keyTeeproxyStatus, keyTeeproxyHost},
	}
	// TeeproxyPkgGoDevBrokenPathCount counts teeproxy requests to pkg.go.dev
	// that return 4xx or 5xx but return 2xx or 3xx on godoc.org.
	TeeproxyPkgGoDevBrokenPathCount = &view.View{
		Name:        "go-discovery/teeproxy/pkgGoDev-brokenPath",
		Measure:     teeproxyPkgGoDevBrokenPaths,
		Aggregation: view.Count(),
		Description: "Count of broken paths in pkg.go.dev",
		TagKeys:     []tag.Key{keyTeeproxyStatus, keyTeeproxyHost, keyTeeproxyPath},
	}
)

// NewServer returns a new Server struct with preconfigured settings.
//
// The server is rate limited and allows events up to a rate of "Rate" and
// a burst of "Burst".
//
// The server also implements the circuit breaker pattern and maintains a
// breaker for each host. Each breaker can be in one of three states: green,
// yellow, or red.
//
// In the green state, the breaker remains green until it encounters a time
// window of length "GreenInterval" where there are more than of "FailsToRed"
// failures and a failureRatio of more than "FailureThreshold", in which case
// the state becomes red.
//
// In the red state, the breaker halts all requests and waits for a timeout
// period before shifting to the yellow state.
//
// In the yellow state, the breaker allows the first "SuccsToGreen" requests.
// If any of these fail, the state reverts to red.
// Otherwise, the state becomes green again.
//
// The timeout period is initially set to "MinTimeout" when the breaker shifts
// from green to yellow. By default, the timeout period is doubled each time
// the breaker fails to shift from the yellow state to the green state and is
// capped at "MaxTimeout".
func NewServer(config Config) (_ *Server, err error) {
	defer derrors.Wrap(&err, "NewServer")
	var breakers = make(map[string]*breaker.Breaker)
	for _, host := range config.Hosts {
		if host == "" {
			return nil, errors.New("host cannot be empty")
		}
		b, err := breaker.New(config.BreakerConfig)
		if err != nil {
			return nil, err
		}
		breakers[host] = b
	}
	var client = http.DefaultClient
	if config.Client != nil {
		client = config.Client
	}

	authKey := config.AuthKey
	if authKey == "" {
		authKey = "auth-key-for-testing"
	}
	return &Server{
		hosts:     config.Hosts,
		client:    client,
		limiter:   rate.NewLimiter(rate.Limit(config.Rate), config.Burst),
		breakers:  breakers,
		authKey:   authKey,
		authValue: config.AuthValue,
	}, nil
}

// ServeHTTP receives requests from godoc.org and forwards them to the
// specified hosts.
// These requests are validated and rate limited before being forwarded. Too
// many error responses returned by pkg.go.dev will cause the server to back
// off temporarily before trying to forward requests to the hosts again.
// ServeHTTP will always reply with StatusOK as long as the request is a valid
// godoc.org request, even if the request could not be processed by the hosts.
// Instead, problems with processing the request by the hosts will logged.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Ignore internal App Engine requests.
	if strings.HasPrefix(r.URL.Path, "/_ah/") {
		// Don't log requests.
		return
	}
	results, status, err := s.doRequest(r)
	if err != nil {
		log.Infof(r.Context(), "teeproxy.Server.ServeHTTP: %v", err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	log.Info(r.Context(), results)
}

func (s *Server) doRequest(r *http.Request) (results map[string]*RequestEvent, status int, err error) {
	defer derrors.Wrap(&err, "doRequest(%q): referer=%q", r.URL.Path, r.Referer())
	ctx := r.Context()
	if status, err = validateTeeProxyRequest(r); err != nil {
		return results, status, err
	}
	gddoEvent, err := getGddoEvent(r)
	if err != nil {
		return results, http.StatusBadRequest, err
	}

	results = map[string]*RequestEvent{
		"godoc.org": gddoEvent,
	}
	if len(s.hosts) > 0 {
		rateLimited := !s.limiter.Allow()
		for _, host := range s.hosts {
			event := &RequestEvent{
				Host: host,
			}

			if rateLimited {
				event.Status = http.StatusTooManyRequests
				event.Error = errors.New("rate limit exceeded")
			} else {
				event = s.doRequestOnHost(ctx, gddoEvent, host)
			}

			if event.Error != nil {
				log.Errorf(r.Context(), "teeproxy.Server.doRequest(%q): %s", host, event.Error)
			}
			results[host] = event
			recordTeeProxyMetric(r.Context(), host, gddoEvent.Path, gddoEvent.Status, event.Status, gddoEvent.Latency, event.Latency)
		}
	}
	return results, http.StatusOK, nil
}

func (s *Server) doRequestOnHost(ctx context.Context, gddoEvent *RequestEvent, host string) *RequestEvent {
	redirectPath := pkgGoDevPath(gddoEvent.Path)
	event := &RequestEvent{
		Host: host,
		Path: redirectPath,
	}

	breaker := s.breakers[host]
	if breaker == nil {
		// This case should never be reached.
		event.Status = http.StatusInternalServerError
		event.Error = errors.New("breaker is nil")
		return event
	}

	if !breaker.Allow() {
		event.Status = statusRedBreaker
		event.Error = errors.New("breaker is red")
		return event
	}

	event = s.makePkgGoDevRequest(ctx, host, pkgGoDevPath(gddoEvent.Path))
	if event.Error != nil {
		return event
	}
	success := event.Status < http.StatusInternalServerError
	breaker.Record(success)
	return event
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
func (s *Server) makePkgGoDevRequest(ctx context.Context, redirectHost, redirectPath string) *RequestEvent {
	var err error
	defer derrors.Wrap(&err, "makePkgGoDevRequest(%q, %q)", redirectHost, redirectPath)
	redirectURL := redirectHost + redirectPath
	event := &RequestEvent{
		Host: redirectHost,
		Path: redirectPath,
		URL:  redirectURL,
	}

	req, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		event.Status = http.StatusInternalServerError
		event.Error = err
		return event
	}
	start := time.Now()
	req.Header.Set(s.authKey, s.authValue)
	resp, err := ctxhttp.Do(ctx, s.client, req)
	if err != nil {
		// Use StatusBadGateway to indicate the upstream error.
		event.Status = http.StatusBadGateway
		event.Error = err
		return event
	}

	event.Status = resp.StatusCode
	event.Latency = time.Since(start)
	return event
}

// recordTeeProxyMetric records the latencies and counts of requests from
// godoc.org and to pkg.go.dev, tagged with the response status code, as well
// as any path that errors on pkg.go.dev but not on godoc.org.
func recordTeeProxyMetric(ctx context.Context, host, path string, gddoStatus, pkgGoDevStatus int, gddoLatency, pkgGoDevLatency time.Duration) {
	gddoL := gddoLatency.Seconds() * 1000
	pkgGoDevL := pkgGoDevLatency.Seconds() * 1000

	// Record latency.
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyTeeproxyStatus, strconv.Itoa(pkgGoDevStatus)),
		tag.Upsert(keyTeeproxyHost, host),
	},
		teeproxyGddoLatency.M(gddoL),
		teeproxyPkgGoDevLatency.M(pkgGoDevL),
	)

	// Record path that returns 4xx or 5xx on pkg.go.dev but returns 2xx or 3xx
	// on godoc.org, excluding rate limiter and circuit breaker errors.
	if pkgGoDevStatus >= 400 && gddoStatus < 400 &&
		pkgGoDevStatus != http.StatusTooManyRequests && pkgGoDevStatus != statusRedBreaker {
		stats.RecordWithTags(ctx, []tag.Mutator{
			tag.Upsert(keyTeeproxyStatus, strconv.Itoa(pkgGoDevStatus)),
			tag.Upsert(keyTeeproxyHost, host),
			tag.Upsert(keyTeeproxyPath, path),
		},
			teeproxyPkgGoDevBrokenPaths.M(1),
		)
	}
}
