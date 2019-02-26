// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "time"

// A Series is a group of modules that share the same base path and are assumed
// to be major-version variants.
type Series struct {
	Name      string
	CreatedAt time.Time
	Modules   []*Module
}

// A Module is a collection of packages that share a common path prefix (the
// module path) and are versioned as a single unit, along with a go.mod file
// listing other required modules.
type Module struct {
	Name      string
	CreatedAt time.Time
	Series    *Series
	Versions  []*Version
}

// A Version is a specific, reproducible build of a module.
type Version struct {
	Module       *Module
	Version      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Synopsis     string
	CommitTime   time.Time
	License      string
	ReadMe       string
	Packages     []*Package
	Dependencies []*Version
	Dependents   []*Version
}

// A Package is a group of one or more Go source files with the same package
// header. Packages are part of a module.
type Package struct {
	Name     string
	Path     string
	Synopsis string
	Version  *Version
}

// A VersionSource is the source of a record in the version logs.
type VersionSource string

const (
	// Version fetched by the Proxy Index Cron
	VersionLogProxyIndex = VersionSource("proxy-index")

	// Version requested by user from Frontend
	VersionLogFrontend = VersionSource("frontend")
)

func (vs VersionSource) String() string {
	return string(vs)
}

// A Version Log is a record of a version that was
// (1) fetched from the module proxy index,
// (2) requested by user from the frontend
// The Name and Version may not necessarily be valid module versions (for example, if a
// user requests a module or version that does not exist).
type VersionLog struct {
	Name      string
	Version   string
	CreatedAt time.Time
	Source    VersionSource
	Error     string
}
