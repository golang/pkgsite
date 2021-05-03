// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"sort"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/derrors"
)

// SymbolSection is the documentation section where a symbol appears.
type SymbolSection string

const (
	SymbolSectionConstants SymbolSection = "Constants"
	SymbolSectionVariables SymbolSection = "Variables"
	SymbolSectionFunctions SymbolSection = "Functions"
	SymbolSectionTypes     SymbolSection = "Types"
)

// SymbolKind is the type of a symbol.
type SymbolKind string

const (
	SymbolKindConstant SymbolKind = "Constant"
	SymbolKindVariable SymbolKind = "Variable"
	SymbolKindFunction SymbolKind = "Function"
	SymbolKindType     SymbolKind = "Type"
	SymbolKindField    SymbolKind = "Field"
	SymbolKindMethod   SymbolKind = "Method"
)

// Symbol is an element in the package API. A symbol can be a constant,
// variable, function, or type.
type Symbol struct {
	SymbolMeta

	// Children contain the child symbols for this symbol. This will
	// only be populated when the SymbolType is "Type". For example, the
	// children of net/http.Handler are FileServer, NotFoundHandler,
	// RedirectHandler, StripPrefix, and TimeoutHandler. Each child
	// symbol will have ParentName set to the Name of this type.
	Children []*SymbolMeta

	// GOOS specifies the execution operating system where the symbol appears.
	GOOS string

	// GOARCH specifies the execution architecture where the symbol appears.
	GOARCH string
}

// SymbolMeta is the metadata for an element in the package API. A symbol can
// be a constant, variable, function, or type.
type SymbolMeta struct {
	// Name is the name of the symbol.
	Name string

	// Synopsis is the one line description of the symbol as displayed
	// in the package documentation.
	Synopsis string

	// Section is the section that a symbol appears in.
	Section SymbolSection

	// Kind is the type of a symbol, which is either a constant, variable,
	// function, type, field or method.
	Kind SymbolKind

	// ParentName if name of the parent type if available, otherwise
	// the empty string. For example, the parent type for
	// net/http.FileServer is Handler.
	ParentName string
}

// SymbolHistory represents the history for when a symbol name was first added
// to a package.
type SymbolHistory struct {
	// m is a map of version to name to SymbolMeta to UnitSymbol.
	// SymbolMeta is stored as a distinct key from name, since it is possible
	// for a symbol in the same version for different build contexts to have
	// different SymbolMeta. For example:
	// https://pkg.go.dev/syscall@go1.16.3#CloseOnExec has function signature:
	// func CloseOnExec(fd int)
	//
	// versus
	// https://pkg.go.dev/syscall?GOOS=windows#CloseOnExec has function
	// signature:
	// func CloseOnExec(fd Handle)
	m map[string]map[string]map[SymbolMeta]*UnitSymbol
}

// NewSymbolHistory returns a new *SymbolHistory.
func NewSymbolHistory() *SymbolHistory {
	return &SymbolHistory{
		m: map[string]map[string]map[SymbolMeta]*UnitSymbol{},
	}
}

// SymbolsAtVersion returns a map of name to SymbolMeta to UnitSymbol for a
// given version.
func (sh *SymbolHistory) SymbolsAtVersion(v string) map[string]map[SymbolMeta]*UnitSymbol {
	return sh.m[v]
}

// Versions returns an array of the versions in versionToNameToUnitSymbol, sorted by
// increasing semver.
func (sh *SymbolHistory) Versions() []string {
	var orderdVersions []string
	for v := range sh.m {
		orderdVersions = append(orderdVersions, v)
	}
	sort.Slice(orderdVersions, func(i, j int) bool {
		return semver.Compare(orderdVersions[i], orderdVersions[j]) == -1
	})
	return orderdVersions
}

// GetSymbol returns the unit symbol for a given name, version and build context.
func (sh *SymbolHistory) GetSymbol(name, v string, build BuildContext) (_ *UnitSymbol, err error) {
	defer derrors.Wrap(&err, "GetSymbol(%q, %q, %v)", name, v, build)
	sav, ok := sh.m[v]
	if !ok {
		return nil, fmt.Errorf("version %q could not be found: %q", v, name)
	}
	stu, ok := sav[name]
	if !ok {
		return nil, fmt.Errorf("symbol %q could not be found at version %q", name, v)
	}
	for _, us := range stu {
		if us.SupportsBuild(build) {
			return us, nil
		}
	}
	return nil, fmt.Errorf("symbol %q does not have build %v at version %q", name, build, v)
}

// AddSymbol adds the given symbol to SymbolHistory.
func (sh *SymbolHistory) AddSymbol(sm SymbolMeta, v string, build BuildContext) {
	sav, ok := sh.m[v]
	if !ok {
		sav = map[string]map[SymbolMeta]*UnitSymbol{}
		sh.m[v] = sav
	}
	stu, ok := sav[sm.Name]
	if !ok {
		stu = map[SymbolMeta]*UnitSymbol{}
		sh.m[v][sm.Name] = stu
	}
	us, ok := stu[sm]
	if !ok {
		us = &UnitSymbol{
			SymbolMeta: sm,
		}
		sh.m[v][sm.Name][sm] = us
	}
	us.AddBuildContext(build)
}

// UnitSymbol represents a symbol that is part of a unit.
type UnitSymbol struct {
	SymbolMeta

	// builds are the build contexts that apply to this symbol.
	builds map[BuildContext]bool
}

// BuildContexts returns the build contexts for this UnitSymbol.
func (us *UnitSymbol) BuildContexts() []BuildContext {
	var builds []BuildContext
	for b := range us.builds {
		builds = append(builds, b)
	}
	sort.Slice(builds, func(i, j int) bool {
		return builds[i].GOOS < builds[j].GOOS
	})
	return builds
}

// AddBuildContext adds a build context supported by this UnitSymbol.
func (us *UnitSymbol) AddBuildContext(build BuildContext) {
	if us.builds == nil {
		us.builds = map[BuildContext]bool{}
	}
	if build != BuildContextAll {
		us.builds[build] = true
		return
	}
	for _, b := range BuildContexts {
		us.builds[b] = true
	}
}

// SupportsBuild reports whether the provided build is supported by this
// UnitSymbol. If the build is BuildContextAll, this is interpreted as this
// unit symbol supports at least one build context.
func (us *UnitSymbol) SupportsBuild(build BuildContext) bool {
	if build == BuildContextAll {
		return len(us.builds) > 0
	}
	return us.builds[build]
}

// InAll reports whether the unit symbol supports all build contexts.
func (us *UnitSymbol) InAll() bool {
	return len(us.builds) == len(BuildContexts)
}

// RemoveBuildContexts removes all of the build contexts associated with this
// unit symbol.
func (us *UnitSymbol) RemoveBuildContexts() {
	us.builds = map[BuildContext]bool{}
}
