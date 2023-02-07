// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package experiment provides functionality for experiments.
package experiment

import (
	"context"
)

type contextKey struct{}

// Set is the set of experiments that are enabled for a request.
type Set struct {
	set map[string]bool
}

// Active returns a list of all the active experiments in s.
func (s *Set) Active() []string {
	if s == nil {
		return nil
	}
	var es []string
	for e := range s.set {
		es = append(es, e)
	}
	return es
}

// FromContext returns the set of experiments enabled for the context.
func FromContext(ctx context.Context) *Set {
	s, _ := ctx.Value(contextKey{}).(*Set)
	return s
}

// NewSet creates a new experiment.Set with the data provided.
func NewSet(experimentNames ...string) *Set {
	set := map[string]bool{}
	for _, e := range experimentNames {
		set[e] = true
	}
	return &Set{set: set}
}

// NewContext stores a set constructed from the provided experiment names in the context.
func NewContext(ctx context.Context, experimentNames ...string) context.Context {
	return context.WithValue(ctx, contextKey{}, NewSet(experimentNames...))
}

// IsActive reports whether an experiment is active for this set.
func IsActive(ctx context.Context, experiment string) bool {
	return FromContext(ctx).IsActive(experiment)
}

// IsActive reports whether an experiment is active for this set.
func (s *Set) IsActive(experiment string) bool {
	if s == nil {
		return false
	}
	return s.set[experiment]
}
