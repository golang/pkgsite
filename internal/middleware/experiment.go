// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"sync"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/experiment"
	"golang.org/x/discovery/internal/log"
)

const experimentQueryParamKey = "experiment"

// An Experimenter contains information about active experiments from the
// experiment source.
type Experimenter struct {
	es        internal.ExperimentSource
	logger    Logger
	pollEvery time.Duration
	mu        sync.Mutex
	snapshot  []*internal.Experiment
}

// NewExperimenter returns an Experimenter for use in the middleware. The
// experimenter regularly polls for updates to the snapshot in the background.
func NewExperimenter(ctx context.Context, pollEvery time.Duration, es internal.ExperimentSource, lg Logger) (_ *Experimenter, err error) {
	defer derrors.Wrap(&err, "middleware.NewExperimenter")
	e := &Experimenter{
		es:        es,
		logger:    lg,
		pollEvery: pollEvery,
	}
	if err := e.loadNextSnapshot(ctx); err != nil {
		return nil, err
	}
	go e.pollUpdates(ctx)
	return e, nil
}

// Experiment returns a new Middleware that sets active experiments for each
// incoming request.
func Experiment(e *Experimenter) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.ServeHTTP(w, e.setExperimentsForRequest(r))
		})
	}
}

// setExperimentsForRequest sets the experiments for a given request.
// Experiments should be stable for a given IP address.
func (e *Experimenter) setExperimentsForRequest(r *http.Request) *http.Request {
	e.mu.Lock()
	defer e.mu.Unlock()

	set := map[string]bool{}
	for _, exp := range e.snapshot {
		if shouldSetExperiment(r, exp) {
			set[exp.Name] = true
		}
	}
	return r.WithContext(experiment.NewContext(r.Context(), set))
}

// pollUpdates polls the experiment source for updates to the snapshot, until
// e.closeChan is closed.
func (e *Experimenter) pollUpdates(ctx context.Context) {
	ticker := time.NewTicker(e.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctx2, cancel := context.WithTimeout(ctx, e.pollEvery)
			if err := e.loadNextSnapshot(ctx2); err != nil {
				log.Error(ctx, err)
			}
			cancel()
		}
	}
}

// loadNextSnapshot loads and sets the current state of experiments from the
// experiment source.
func (e *Experimenter) loadNextSnapshot(ctx context.Context) (err error) {
	defer derrors.Wrap(&err, "loadNextSnapshot")
	snapshot, err := e.es.GetExperiments(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.snapshot = snapshot
	e.mu.Unlock()
	return nil
}

// shouldSetExperiment reports whether a given request should be enrolled in
// the experiment, based on the ip. e.Name, and e.Rollout.
//
// Requests from empty ip addresses are never enrolled.
// All requests from the same IP will be enrolled in the same set of
// experiments.
func shouldSetExperiment(r *http.Request, e *internal.Experiment) bool {
	keys := r.URL.Query()[experimentQueryParamKey]
	for _, k := range keys {
		if k == e.Name {
			return true
		}
	}
	if e.Rollout == 0 {
		return false
	}
	ip := ipKey(r.Header.Get("X-Forwarded-For"))
	if ip == "" {
		return false
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%s %s", ip, e.Name)
	return uint(h.Sum32())%(100/e.Rollout) == 0
}
