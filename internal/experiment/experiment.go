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

// FromContent returns the set of experiments enabled for the content.
func FromContext(ctx context.Context) *Set {
	s, _ := ctx.Value(contextKey{}).(*Set)
	return s
}

// NewContext stores the provided experiment set in the context.
func NewContext(ctx context.Context, set map[string]bool) context.Context {
	return context.WithValue(ctx, contextKey{}, &Set{set: set})
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
