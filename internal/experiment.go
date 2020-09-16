// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

import (
	"context"
)

const (
	ExperimentAutocomplete  = "autocomplete"
	ExperimentFrontendFetch = "frontend-fetch"
	ExperimentSidenav       = "sidenav"
	ExperimentUnitPage      = "unit-page"
	ExperimentUseUnits      = "use-units"
	ExperimentUsePathInfo   = "use-path-info"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentAutocomplete:  "Enable autocomplete with search.",
	ExperimentFrontendFetch: "Enable ability to fetch a package that doesn't exist on pkg.go.dev.",
	ExperimentSidenav:       "Display documentation index on the left sidenav.",
	ExperimentUnitPage:      "Enable the redesigned details page.",
	ExperimentUseUnits:      "Read from paths, documentation, readmes, and package_imports tables.",
	ExperimentUsePathInfo:   "Check the paths table if a path exists, as opposed to the packages or modules table.",
}

// Experiment holds data associated with an experimental feature for frontend
// or worker.
type Experiment struct {
	// Name is the name of the feature.
	Name string

	// Rollout is the percentage of requests enrolled in the experiment.
	Rollout uint

	// Description provides a description of the experiment.
	Description string
}

// ExperimentSource is the interface used by the middleware to interact with
// experiments data.
type ExperimentSource interface {
	// GetExperiments fetches active experiments from the
	// ExperimentSource.
	GetExperiments(ctx context.Context) ([]*Experiment, error)
}

// LocalExperimentSource is used when developing locally using the direct proxy
// mode.
type LocalExperimentSource struct {
	experiments []*Experiment
}

// NewLocalExperimentSource returns a LocalExperimentSource with the provided experiments.
func NewLocalExperimentSource(experiments []*Experiment) *LocalExperimentSource {
	return &LocalExperimentSource{experiments: experiments}
}

// GetExperiments returns the experiments for the given LocalExperimentSource.
func (e *LocalExperimentSource) GetExperiments(ctx context.Context) ([]*Experiment, error) {
	return e.experiments, nil
}
