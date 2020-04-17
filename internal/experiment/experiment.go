// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

// FromContent returns the set of experiments enabled for the content.
func FromContext(ctx context.Context) *Set {
	s, _ := ctx.Value(contextKey{}).(*Set)
	return s
}

// NewSet creates a new experiment.Set with the data provided.
func NewSet(set map[string]bool) *Set {
	return &Set{set: set}
}

// NewContext stores the provided experiment set in the context.
func NewContext(ctx context.Context, set *Set) context.Context {
	return context.WithValue(ctx, contextKey{}, set)
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
