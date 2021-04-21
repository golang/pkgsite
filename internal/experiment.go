// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentDeprecatedDoc             = "deprecated-doc"
	ExperimentInsertSymbolHistory       = "insert-symbol-history"
	ExperimentReadSymbolHistory         = "read-symbol-history"
	ExperimentSymbolHistoryVersionsPage = "symbol-history-versions-page"
	ExperimentSymbolHistoryMainPage     = "symbol-history-main-page"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentDeprecatedDoc:             "Treat deprecated symbols specially in documentation.",
	ExperimentInsertSymbolHistory:       "Insert data into the symbol_history table.",
	ExperimentReadSymbolHistory:         "Read data from the symbol_history table.",
	ExperimentSymbolHistoryVersionsPage: "Show package API history on the versions page.",
	ExperimentSymbolHistoryMainPage:     "Show package API history on the main unit page.",
}

// Experiment holds data associated with an experimental feature for frontend
// or worker.
type Experiment struct {
	// This struct is used to decode dynamic config (see
	// internal/config/dynconfig). Make sure that changes to this struct are
	// coordinated with the deployment of config files.

	// Name is the name of the feature.
	Name string

	// Rollout is the percentage of requests enrolled in the experiment.
	Rollout uint

	// Description provides a description of the experiment.
	Description string
}
