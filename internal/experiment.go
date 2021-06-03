// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentDeprecatedDoc               = "deprecated-doc"
	ExperimentSkipInsertSymbols           = "skip-insert-symbols"
	ExperimentInsertSymbolSearchDocuments = "insert-symbol-search-documents"
	ExperimentReadSymbolHistory           = "read-symbol-history"
	ExperimentSearchGrouping              = "search-grouping"
	ExperimentStyleGuide                  = "styleguide"
	ExperimentSymbolHistoryMainPage       = "symbol-history-main-page"
	ExperimentSymbolHistoryVersionsPage   = "symbol-history-versions-page"
	ExperimentSymbolSearch                = "symbol-search"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentDeprecatedDoc:               "Treat deprecated symbols specially in documentation.",
	ExperimentSkipInsertSymbols:           "Don't insert data into symbols tables.",
	ExperimentInsertSymbolSearchDocuments: "Insert data into symbol_search_documents.",
	ExperimentReadSymbolHistory:           "Read data from the symbol_history table.",
	ExperimentSearchGrouping:              "Group search results.",
	ExperimentStyleGuide:                  "Enable the styleguide.",
	ExperimentSymbolHistoryMainPage:       "Show package API history on the main unit page.",
	ExperimentSymbolHistoryVersionsPage:   "Show package API history on the versions page.",
	ExperimentSymbolSearch:                "Enable searching for symbols.",
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
