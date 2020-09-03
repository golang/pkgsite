// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal/licenses"
)

// LegacyModuleInfo holds metadata associated with a module.
type LegacyModuleInfo struct {
	ModuleInfo
	LegacyReadmeFilePath string
	LegacyReadmeContents string
}

// LegacyDirectory represents a directory in a module version, and all of the
// packages inside that directory.
type LegacyDirectory struct {
	LegacyModuleInfo
	Path     string
	Packages []*LegacyPackage
}

// A LegacyPackage is a group of one or more Go source files with the same
// package header. LegacyPackages are part of a module.
type LegacyPackage struct {
	Path              string
	Name              string
	Synopsis          string
	IsRedistributable bool
	Licenses          []*licenses.Metadata // metadata of applicable licenses
	Imports           []string
	DocumentationHTML safehtml.HTML
	// The values of the GOOS and GOARCH environment variables used to parse the
	// package.
	GOOS   string
	GOARCH string

	// V1Path is the package path of a package with major version 1 in a given
	// series.
	V1Path string
}

// LegacyVersionedPackage is a LegacyPackage along with its corresponding module
// information.
type LegacyVersionedPackage struct {
	LegacyPackage
	LegacyModuleInfo
}
