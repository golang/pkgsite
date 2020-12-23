// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal contains data used through x/pkgsite.
package internal

const (
	ExperimentGetUnitMetaQuery   = "get-unit-meta-query"
	ExperimentGoldmark           = "goldmark"
	ExperimentNotAtLatest        = "not-at-latest"
	ExperimentReadmeOutline      = "readme-outline"
	ExperimentUnitSidebarDetails = "unit-sidebar-details"
)

// Experiments represents all of the active experiments in the codebase and
// a description of each experiment.
var Experiments = map[string]string{
	ExperimentGetUnitMetaQuery:   "Enable the new get unit meta query, which reads from the paths table.",
	ExperimentGoldmark:           "Enable the usage of rendering markdown using goldmark instead of blackfriday.",
	ExperimentNotAtLatest:        "Enable the display of a 'not at latest' badge.",
	ExperimentReadmeOutline:      "Enable the readme outline in the side nav.",
	ExperimentUnitSidebarDetails: "Enable the details section in the right sidebar.",
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
