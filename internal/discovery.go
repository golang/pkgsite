// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "time"

// A Series is a group of modules that share the same base path and are assumed
// to be major-version variants.
type Series struct {
	Name    string
	Modules []*Module
}

// A Module is a collection of packages that share a common path prefix (the
// module path) and are versioned as a single unit, along with a go.mod file
// listing other required modules.
type Module struct {
	Name     string
	Series   *Series
	Versions []*Version
}

// A Version is a specific, reproducible build of a module.
type Version struct {
	Module       *Module
	Version      string
	Synopsis     string
	CommitTime   time.Time
	License      *License
	ReadMe       *ReadMe
	Packages     []*Package
	Dependencies []*Version
	Dependents   []*Version
}

// A Package is a group of one or more Go source files with the same package
// header. Packages are part of a module.
type Package struct {
	Version  *Version
	Path     string
	Synopsis string
}

// A ReadMe represents the contents of a README file.
type ReadMe struct {
	Version  *Version
	Contents string
}

// A License represents the contents of a LICENSE file.
type License struct {
	Version  *Version
	Type     string
	Contents string
}
