// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/errorreporting"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
)

const experimentQueryParamKey = "experiment"

// A Reporter sends errors to the Error-Reporting service.
type Reporter interface {
	Report(errorreporting.Entry)
}

// An Experimenter contains information about active experiments from the
// experiment source.
type Experimenter struct {
	getExperimentSource func(context.Context) internal.ExperimentSource
	reporter            Reporter
	pollEvery           time.Duration
	mu                  sync.Mutex
	snapshot            []*internal.Experiment
}

// NewExperimenter returns an Experimenter for use in the middleware. The
// experimenter regularly polls for updates to the snapshot in the background.
func NewExperimenter(ctx context.Context, pollEvery time.Duration, esf func(context.Context) internal.ExperimentSource, rep Reporter) (_ *Experimenter, err error) {
	defer derrors.Wrap(&err, "middleware.NewExperimenter")
	e := &Experimenter{
		getExperimentSource: esf,
		reporter:            rep,
		pollEvery:           pollEvery,
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
			r2 := e.setExperimentsForRequest(r)
			h.ServeHTTP(w, r2)
		})
	}
}

// Experiments returns the experiments currently in use.
func (e *Experimenter) Experiments() []*internal.Experiment {
	// Make a copy so the caller can't modify our state.
	e.mu.Lock()
	defer e.mu.Unlock()
	exps := make([]*internal.Experiment, len(e.snapshot))
	for i, x := range e.snapshot {
		// Assume internal.Experiment has no pointers to mutable data.
		nx := *x
		exps[i] = &nx
	}
	return exps
}

// setExperimentsForRequest sets the experiments for a given request.
// Experiments should be stable for a given IP address.
func (e *Experimenter) setExperimentsForRequest(r *http.Request) *http.Request {
	e.mu.Lock()
	defer e.mu.Unlock()

	var exps []string
	for _, exp := range e.snapshot {
		if shouldSetExperiment(r, exp) {
			exps = append(exps, exp.Name)
		}
	}
	exps = append(exps, r.URL.Query()[experimentQueryParamKey]...)
	return r.WithContext(experiment.NewContext(r.Context(), exps...))
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
				if e.reporter != nil {
					e.reporter.Report(errorreporting.Entry{
						Error: fmt.Errorf("loading experiments: %v", err),
					})
				}
			}
			cancel()
		}
	}
}

// loadNextSnapshot loads and sets the current state of experiments from the
// experiment source.
func (e *Experimenter) loadNextSnapshot(ctx context.Context) (err error) {
	defer derrors.Wrap(&err, "loadNextSnapshot")
	es := e.getExperimentSource(ctx)
	if es == nil {
		return errors.New("no ExperimentSource available")
	}
	snapshot, err := es.GetExperiments(ctx)
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
	if e.Rollout == 0 {
		return false
	}
	if e.Rollout >= 100 {
		return true
	}
	ip := ipKey(r.Header.Get("X-Forwarded-For"))
	if ip == "" {
		return false
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%s %s", ip, e.Name)
	return uint(h.Sum32())%100 < e.Rollout
}
