// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
)

const (
	ExperimentAutocomplete       = "autocomplete"
	ExperimentFrontendFetch      = "frontend-fetch"
	ExperimentMasterVersion      = "master-version"
	ExperimentExecutableExamples = "executable-examples"
	ExperimentNewHomepage        = "new-homepage"
	ExperimentSidenav            = "sidenav"
	ExperimentTranslateHTML      = "translate-html"
	ExperimentUseDirectories     = "use-directories"
	ExperimentUsePackageImports  = "use-package-imports"
	ExperimentUsePathInfo        = "use-path-info"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentAutocomplete:       "Enable autocomplete with search.",
	ExperimentFrontendFetch:      "Enable ability to fetch a package that doesn't exist on pkg.go.dev.",
	ExperimentMasterVersion:      "Enable viewing path@master.",
	ExperimentNewHomepage:        "Enable the new hompage.",
	ExperimentExecutableExamples: "Display executable examples with their import statements, so that they are runnable via the Go playground.",
	ExperimentSidenav:            "Display documentation index on the left sidenav.",
	ExperimentTranslateHTML:      "Parse HTML text in READMEs, to properly display images.",
	ExperimentUseDirectories:     "Read from paths, documentation, readmes, and package_imports tables.",
	ExperimentUsePathInfo:        "Check the paths table if a path exists, as opposed to the packages or modules table.",
	ExperimentUsePackageImports:  "Read imports from the package_imports table.",
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
