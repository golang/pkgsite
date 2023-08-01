// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/poller"
)

const experimentQueryParamKey = "experiment"

// ExperimentGetter is the signature of a function that gets experiments.
type ExperimentGetter func(context.Context) ([]*internal.Experiment, error)

// An Experimenter contains information about active experiments from the
// experiment source.
type Experimenter struct {
	p *poller.Poller
}

// NewExperimenter returns an Experimenter for use in the middleware. The
// experimenter regularly polls for updates to the snapshot in the background.
func NewExperimenter(ctx context.Context, pollEvery time.Duration, getter ExperimentGetter, rep derrors.Reporter) (_ *Experimenter, err error) {
	defer derrors.Wrap(&err, "middleware.NewExperimenter")

	initial, err := getter(ctx)
	// If we can't load the initial state, then fail.
	if err != nil {
		return nil, err
	}
	e := &Experimenter{
		p: poller.New(
			initial,
			func(ctx context.Context) (any, error) {
				return getter(ctx)
			},
			func(err error) {
				// Log and report // the error.
				log.Error(ctx, err)
				if rep != nil {
					rep.Report(fmt.Errorf("loading experiments: %v", err), nil, nil)
				}
			}),
	}
	e.p.Start(ctx, pollEvery)
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
	snapshot := e.p.Current().([]*internal.Experiment)
	// We don't need a lock here because e.p.current will be updated
	// without modification.
	exps := make([]*internal.Experiment, len(snapshot))
	for i, x := range snapshot {
		// Assume internal.Experiment has no pointers to mutable data.
		nx := *x
		exps[i] = &nx
	}
	return exps
}

// setExperimentsForRequest sets the experiments for a given request.
// Experiments should be stable for a given IP address.
func (e *Experimenter) setExperimentsForRequest(r *http.Request) *http.Request {
	snapshot := e.p.Current().([]*internal.Experiment)
	var exps []string
	for _, exp := range snapshot {
		if shouldSetExperiment(r, exp) {
			exps = append(exps, exp.Name)
		}
	}
	exps = append(exps, r.URL.Query()[experimentQueryParamKey]...)
	return r.WithContext(experiment.NewContext(r.Context(), exps...))
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
