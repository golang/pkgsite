// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentDeprecatedDoc               = "deprecated-doc"
	ExperimentEnableStdFrontendFetch      = "enable-std-frontend-fetch"
	ExperimentInsertSymbolSearchDocuments = "insert-symbol-search-documents"
	ExperimentNewUnitLayout               = "new-unit-layout"
	ExperimentSearchGrouping              = "search-grouping"
	ExperimentSearchIncrementally         = "search-incrementally"
	ExperimentSkipInsertSymbols           = "skip-insert-symbols"
	ExperimentStyleGuide                  = "styleguide"
	ExperimentSymbolHistoryMainPage       = "symbol-history-main-page"
	ExperimentSymbolHistoryVersionsPage   = "symbol-history-versions-page"
	ExperimentSymbolSearch                = "symbol-search"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentDeprecatedDoc:               "Treat deprecated symbols specially in documentation.",
	ExperimentEnableStdFrontendFetch:      "Enable frontend fetching for module std.",
	ExperimentInsertSymbolSearchDocuments: "Insert data into symbol_search_documents and documentation_symbols.",
	ExperimentNewUnitLayout:               "Enable the new layout on the unit page.",
	ExperimentSearchGrouping:              "Group search results.",
	ExperimentSearchIncrementally:         "Use incremental query for search results.",
	ExperimentSkipInsertSymbols:           "Don't insert data into symbols tables.",
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
