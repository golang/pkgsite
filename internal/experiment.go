// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentCommandTOC                = "command-toc"
	ExperimentInsertSymbolHistory       = "insert-symbol-history"
	ExperimentInsertSymbols             = "insert-symbols"
	ExperimentInteractivePlayground     = "interactive-playground"
	ExperimentNotAtLatest               = "not-at-latest"
	ExperimentRetractions               = "retractions"
	ExperimentSymbolHistoryVersionsPage = "symbol-history-versions-page"
	ExperimentUnitMetaWithLatest        = "unit-meta-with-latest"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentCommandTOC:                "Enable the table of contents for command documention pages.",
	ExperimentInsertSymbolHistory:       "Insert symbol history data into the symbol_history table.",
	ExperimentInsertSymbols:             "Insert data into symbols, package_symbols, and documentation_symbols.",
	ExperimentInteractivePlayground:     "Enable interactive example playground on the unit page.",
	ExperimentNotAtLatest:               "Enable the display of a 'not at latest' badge.",
	ExperimentRetractions:               "Retrieve and display retraction and deprecation information.",
	ExperimentSymbolHistoryVersionsPage: "Show package API history on the versions page.",
	ExperimentUnitMetaWithLatest:        "Use latest-version information for GetUnitMeta.",
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
