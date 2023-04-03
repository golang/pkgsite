// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import "time"

var (
	idDir           = "ID"
	dbEndpoint      = "index/db"
	modulesEndpoint = "index/modules"
	vulnsEndpoint   = "index/vulns"
)

// DBMeta contains metadata about the database itself.
type DBMeta struct {
	// Modified is the time the database was last modified, calculated
	// as the most recent time any single OSV entry was modified.
	Modified time.Time `json:"modified"`
}

// ModuleMeta contains metadata about a Go module that has one
// or more vulnerabilities in the database.
//
// Found in the "index/modules" endpoint of the vulnerability database.
type ModuleMeta struct {
	// Path is the module path.
	Path string `json:"path"`
	// Vulns is a list of vulnerabilities that affect this module.
	Vulns []ModuleVuln `json:"vulns"`
}

// ModuleVuln contains metadata about a vulnerability that affects
// a certain module.
type ModuleVuln struct {
	// ID is a unique identifier for the vulnerability.
	// The Go vulnerability database issues IDs of the form
	// GO-<YEAR>-<ENTRYID>.
	ID string `json:"id"`
	// Modified is the time the vuln was last modified.
	Modified time.Time `json:"modified"`
	// Fixed is the latest version that introduces a fix for the
	// vulnerability, in SemVer 2.0.0 format, with no leading "v" prefix.
	Fixed string `json:"fixed,omitempty"`
}

// VulnMeta contains metadata about a vulnerability in the database.
//
// Found in the "index/vulns" endpoint of the vulnerability database.
type VulnMeta struct {
	// ID is a unique identifier for the vulnerability.
	// The Go vulnerability database issues IDs of the form
	// GO-<YEAR>-<ENTRYID>.
	ID string `json:"id"`
	// Modified is the time the vulnerability was last modified.
	Modified time.Time `json:"modified"`
	// Aliases is a list of IDs for the same vulnerability in other
	// databases.
	Aliases []string `json:"aliases,omitempty"`
}
