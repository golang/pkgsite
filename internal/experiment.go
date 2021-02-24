// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentCommandTOC            = "command-toc"
	ExperimentDirectoryTree         = "directory-tree"
	ExperimentInsertSymbols         = "insert-symbols"
	ExperimentInsertSymbolHistory   = "insert-symbol-history"
	ExperimentInteractivePlayground = "interactive-playground"
	ExperimentNotAtLatest           = "not-at-latest"
	ExperimentRedirectedFromBanner  = "redirected-from-banner"
	ExperimentRetractions           = "retractions"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentCommandTOC:            "Enable the table of contents for command documention pages.",
	ExperimentDirectoryTree:         "Enable the directory tree layout on the unit page.",
	ExperimentInsertSymbols:         "Insert data into symbols, package_symbols, and documentation_symbols.",
	ExperimentInsertSymbolHistory:   "Insert symbol history data into the symbol_history table.",
	ExperimentInteractivePlayground: "Enable interactive example playground on the unit page.",
	ExperimentNotAtLatest:           "Enable the display of a 'not at latest' badge.",
	ExperimentRedirectedFromBanner:  "Display banner with path that request was redirected from.",
	ExperimentRetractions:           "Retrieve and display retraction and deprecation information.",
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
