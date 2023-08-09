// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
)

func setExperimentsFromQueryParam(ctx context.Context, r *http.Request) context.Context {
	if err := r.ParseForm(); err != nil {
		log.Errorf(ctx, "ParseForm: %v", err)
		return ctx
	}
	return newContextFromExps(ctx, r.Form["exp"])
}

// newContextFromExps adds and removes experiments from the context's experiment
// set, creates a new set with the changes, and returns a context with the new
// set. Each string in expMods can be either an experiment name, which means
// that the experiment should be added, or "!" followed by an experiment name,
// meaning that it should be removed.
func newContextFromExps(ctx context.Context, expMods []string) context.Context {
	var (
		exps   []string
		remove = map[string]bool{}
	)
	set := experiment.FromContext(ctx)
	for _, exp := range expMods {
		if strings.HasPrefix(exp, "!") {
			exp = exp[1:]
			if set.IsActive(exp) {
				remove[exp] = true
			}
		} else if !set.IsActive(exp) {
			exps = append(exps, exp)
		}
	}
	if len(exps) == 0 && len(remove) == 0 {
		return ctx
	}
	for _, a := range set.Active() {
		if !remove[a] {
			exps = append(exps, a)
		}
	}
	return experiment.NewContext(ctx, exps...)
}
