// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package teeproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal/breaker"
)

func TestPkgGoDevPath(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{
			path: "/-/about",
			want: "/about",
		},
		{
			path: "/net/http",
			want: "/net/http",
		},
		{
			path: "/",
			want: "/",
		},
		{
			path: "",
			want: "",
		},
	} {
		if got := pkgGoDevPath(test.path); got != test.want {
			t.Fatalf("pkgGoDevPath(%q) = %q; want = %q", test.path, got, test.want)
		}
	}
}

func TestPkgGoDevRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer ts.Close()

	ctx := context.Background()
	s := newTestServer(Config{})

	got := s.makePkgGoDevRequest(ctx, ts.URL, "")
	if got.Error != nil {
		t.Fatal(got.Error)
	}

	want := &RequestEvent{
		Host:   ts.URL,
		URL:    ts.URL,
		Status: http.StatusOK,
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(RequestEvent{}, "Latency")); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestGetGddoEvent(t *testing.T) {
	for _, test := range []struct {
		gddoEvent *RequestEvent
	}{
		{

			&RequestEvent{
				Host:    "godoc.org",
				URL:     "https://godoc.org/net/http",
				Latency: 100,
				Status:  200,
			},
		},
	} {
		requestBody, err := json.Marshal(test.gddoEvent)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/", bytes.NewBuffer(requestBody))
		gotEvent, err := getGddoEvent(r)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(test.gddoEvent, gotEvent); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestServerHandler(t *testing.T) {
	for _, test := range []struct {
		name         string
		serverConfig Config
		handler      http.Handler
		steps        []interface{}
	}{
		{
			name:         "rate limiter permits requests below cap",
			serverConfig: Config{Rate: 20, Burst: 20},
			handler:      alwaysHandler{http.StatusOK},
			steps: []interface{}{
				request{15, http.StatusOK},
			},
		},
		{
			name:         "rate limiter permits requests up to cap",
			serverConfig: Config{Rate: 20, Burst: 20},
			handler:      alwaysHandler{http.StatusOK},
			steps: []interface{}{
				request{20, http.StatusOK},
			},
		},
		{
			name:         "rate limiter drops requests over cap",
			serverConfig: Config{Rate: 5, Burst: 5},
			handler:      alwaysHandler{http.StatusOK},
			steps: []interface{}{
				request{5, http.StatusOK},
				request{6, http.StatusTooManyRequests},
			},
		},
		{
			name:         "rate limiter permits requests after replenishing",
			serverConfig: Config{Rate: 2, Burst: 2},
			handler:      alwaysHandler{http.StatusOK},
			steps: []interface{}{
				request{2, http.StatusOK},
				request{3, http.StatusTooManyRequests},
				wait{1 * time.Second},
				request{2, http.StatusOK},
				request{3, http.StatusTooManyRequests},
			},
		},
		{
			name:         "green breaker passes requests",
			serverConfig: Config{Rate: 100, Burst: 100},
			handler:      alwaysHandler{http.StatusOK},
			steps: []interface{}{
				checkState{breaker.Green},
				request{25, http.StatusOK},
				checkState{breaker.Green},
				request{25, http.StatusOK},
				checkState{breaker.Green},
			},
		},
		{
			name: "green breaker resets failure count after interval",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed:    5,
				GreenInterval: 100 * time.Millisecond,
			}},
			handler: alwaysHandler{http.StatusServiceUnavailable},
			steps: []interface{}{
				checkState{breaker.Green},
				request{5, http.StatusServiceUnavailable},
				checkState{breaker.Green},
				wait{150 * time.Millisecond},
				checkState{breaker.Green},
				request{5, http.StatusServiceUnavailable},
				checkState{breaker.Green},
				request{1, http.StatusServiceUnavailable},
				checkState{breaker.Red},
			},
		},
		{
			name: "breaker changes to red state and blocks requests",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed: 5,
				MinTimeout: 1 * time.Second,
			}},
			handler: alwaysHandler{http.StatusServiceUnavailable},
			steps: []interface{}{
				checkState{breaker.Green},
				request{6, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				request{20, statusRedBreaker},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				request{20, statusRedBreaker},
				checkState{breaker.Red},
			},
		},
		{
			name: "breaker changes to yellow state",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed: 5,
				MinTimeout: 100 * time.Millisecond,
			}},
			handler: &handler{6, http.StatusServiceUnavailable, alwaysHandler{http.StatusOK}},
			steps: []interface{}{
				request{6, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				request{20, statusRedBreaker},
				checkState{breaker.Red},
				wait{150 * time.Millisecond},
				checkState{breaker.Yellow},
				request{9, http.StatusOK},
				checkState{breaker.Yellow},
			},
		},
		{
			name: "breaker changes to green state again",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed:   5,
				MinTimeout:   100 * time.Millisecond,
				SuccsToGreen: 10,
			}},
			handler: &handler{6, http.StatusServiceUnavailable, alwaysHandler{http.StatusOK}},
			steps: []interface{}{
				request{6, http.StatusServiceUnavailable},
				request{20, statusRedBreaker},
				wait{150 * time.Millisecond},
				request{9, http.StatusOK},
				checkState{breaker.Yellow},
				request{1, http.StatusOK},
				checkState{breaker.Green},
				request{5, http.StatusOK},
			},
		},
		{
			name: "breaker reverts to red state and doubles timeout period on repeated failures",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed: 5,
				MinTimeout: 100 * time.Millisecond,
				MaxTimeout: 400 * time.Millisecond,
			}},
			handler: alwaysHandler{http.StatusServiceUnavailable},
			steps: []interface{}{
				request{6, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				checkState{breaker.Yellow},
				request{1, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				checkState{breaker.Yellow},
				request{1, http.StatusServiceUnavailable},
				checkState{breaker.Red},
			},
		},
		{
			name: "breaker timeout period does not exceed maxTimeout",
			serverConfig: Config{BreakerConfig: breaker.Config{
				FailsToRed: 5,
				MinTimeout: 100 * time.Millisecond,
				MaxTimeout: 100 * time.Millisecond,
			}},
			handler: alwaysHandler{http.StatusServiceUnavailable},
			steps: []interface{}{
				request{6, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				checkState{breaker.Yellow},
				request{1, http.StatusServiceUnavailable},
				checkState{breaker.Red},
				wait{100 * time.Millisecond},
				checkState{breaker.Yellow},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mockPkgGoDevServer := httptest.NewServer(test.handler)
			defer mockPkgGoDevServer.Close()
			test.serverConfig.Hosts = []string{mockPkgGoDevServer.URL}
			server := newTestServer(test.serverConfig)
			executeSteps(t, server, mockPkgGoDevServer.URL, test.steps)
		})
	}
}

func executeSteps(t *testing.T, server *Server, pkgGoDevURL string, steps []interface{}) {
	for s, step := range steps {
		switch step := step.(type) {
		case request:
			for i := 0; i < step.repeat; i++ {
				event := makePostRequest(t, server, pkgGoDevURL)
				if event.Status != step.expectedStatus {
					t.Errorf("step %d request %d: got status %d, want %d", s, i, event.Status, step.expectedStatus)
				}
			}
		case wait:
			time.Sleep(step.wait)
		case checkState:
			if server.breakers[pkgGoDevURL].State() != step.expectedState {
				t.Errorf("step %d: got %s, want %s", s, server.breakers[pkgGoDevURL].State().String(), step.expectedState.String())
			}
		default:
			panic("invalid step type")
		}
	}
}

// TestHandler tests that the handler struct returns
// the correct status codes.
func TestHandler(t *testing.T) {
	h := &handler{5, 500, alwaysHandler{200}}
	s := httptest.NewServer(h)
	defer s.Close()

	for i := 0; i < 5; i++ {
		resp, err := http.PostForm(s.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 500 {
			t.Errorf("request %d: got status %d, want %d", i, resp.StatusCode, 500)
		}
	}

	for i := 0; i < 20; i++ {
		resp, err := http.PostForm(s.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("request %d: got status %d, want %d", i, resp.StatusCode, 200)
		}
	}
}

func makePostRequest(t *testing.T, server *Server, pkgGoDevURL string) *RequestEvent {
	gddoEvent := &RequestEvent{
		Host: "godoc.org",
		URL:  "https://godoc.org/net/http",
	}
	requestBody, err := json.Marshal(gddoEvent)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", bytes.NewBuffer(requestBody))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	results, _, err := server.doRequest(r)
	if err != nil || results == nil {
		t.Fatalf("doRequest = %v: %v", results, err)
	}
	event := results[pkgGoDevURL]
	if event == nil {
		t.Fatalf("results[%q] = %v", pkgGoDevURL, event)
	}
	return event
}

// newTestServer is like NewServer, but with default values for easier testing.
func newTestServer(config Config) *Server {
	// Set default values.
	if config.Rate <= 0 {
		config.Rate = 50
	}
	if config.Burst <= 0 {
		config.Burst = 50
	}
	if config.BreakerConfig.FailsToRed <= 0 {
		config.BreakerConfig.FailsToRed = 10
	}
	if config.BreakerConfig.FailureThreshold <= 0 {
		config.BreakerConfig.FailureThreshold = 0.5
	}
	if config.BreakerConfig.GreenInterval <= 0 {
		config.BreakerConfig.GreenInterval = 200 * time.Millisecond
	}
	if config.BreakerConfig.MinTimeout <= 0 {
		config.BreakerConfig.MinTimeout = 100 * time.Millisecond
	}
	if config.BreakerConfig.MaxTimeout <= 0 {
		config.BreakerConfig.MaxTimeout = 400 * time.Millisecond
	}
	if config.BreakerConfig.SuccsToGreen <= 0 {
		config.BreakerConfig.SuccsToGreen = 20
	}

	server, _ := NewServer(config)
	return server
}

// handler returns statusCode for the first n requests
// and uses innerHandler to serve the remaining requests.
type handler struct {
	n            int
	statusCode   int
	innerHandler http.Handler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.n <= 0 {
		h.innerHandler.ServeHTTP(w, r)
		return
	}
	h.n--
	w.WriteHeader(h.statusCode)
}

type alwaysHandler struct {
	statusCode int
}

func (h alwaysHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(h.statusCode)
}

type request struct {
	repeat         int
	expectedStatus int
}

type wait struct {
	wait time.Duration
}

type checkState struct {
	expectedState breaker.State
}
