// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
)

// Symbol is an element in the package API. A symbol can be a constant,
// variable, function, type, field or method.
type Symbol struct {
	// Name is name of the symbol. At a given package version, name must be
	// unique.
	Name string

	// Synopsis is the one line description of the symbol that is displayed.
	Synopsis string

	// Link is the link to the symbol name on pkg.go.dev.
	Link string

	// Section is the section that a symbol appears in.
	Section internal.SymbolSection

	// Kind is the type of a symbol, which is either a constant, variable,
	// function, type, field or method.
	Kind internal.SymbolKind

	// Children contain the child symbols for this symbol. This will
	// only be populated when the SymbolType is "Type". For example, the
	// children of net/http.Handler are FileServer, NotFoundHandler,
	// RedirectHandler, StripPrefix, and TimeoutHandler. Each child
	// symbol will have ParentName set to the Name of this type.
	Children []*Symbol

	// Builds lists all of the build contexts supported by the symbol, it is
	// only available for limited set of builds. If the symbol supports all
	// build contexts, Builds will be nil.
	Builds []string

	// New indicates that the symbol is new as of the version where it is
	// present. For example, if type Client was introduced in v1.0.0 and
	// Client.Timeout was introduced in v1.1.0, New will be false for Client
	// and true for Client.Timeout if this Symbol corresponds to v1.1.0.
	New bool
}

// symbolsForVersions returns an array of symbols for use in the VersionSummary
// of the specified version.
func symbolsForVersion(pkgURLPath string, symbolsAtVersion map[string]*internal.UnitSymbol) [][]*Symbol {
	nameToSymbol := map[string]*Symbol{}
	for _, us := range symbolsAtVersion {
		builds := symbolBuilds(us)
		s, ok := nameToSymbol[us.Name]
		if !ok {
			s = &Symbol{
				Name:     us.Name,
				Synopsis: us.Synopsis,
				Link:     symbolLink(pkgURLPath, us.Name, us.BuildContexts()),
				Section:  us.Section,
				Kind:     us.Kind,
				New:      true,
				Builds:   builds,
			}
		} else if !s.New && us.Kind == internal.SymbolKindType {
			// It's possible that a symbol was already created if this is a parent
			// symbol and we called addSymbol on the child symbol first. In that
			// case, a parent symbol would have been created where s.New is set to
			// false and s.Synopsis is set to the one created in createParent.
			// Update those fields instead of overwritting the struct, since the
			// struct's Children field would have already been populated.
			s.New = true
			s.Synopsis = us.Synopsis
		}
		if us.ParentName == us.Name {
			// s is not a child symbol of a type, so add it to the map and
			// continue.
			nameToSymbol[us.Name] = s
			continue
		}

		// s is a child symbol of a parent type, so append it to the Children field
		// of the parent type.
		parent, ok := nameToSymbol[us.ParentName]
		if !ok {
			parent = createParent(us, pkgURLPath)
			nameToSymbol[us.ParentName] = parent
		}
		if len(parent.Builds) == len(s.Builds) {
			s.Builds = nil
		}
		parent.Children = append(parent.Children, s)
	}
	return sortSymbols(nameToSymbol)
}

func symbolLink(pkgURLPath, name string, builds []internal.BuildContext) string {
	if len(builds) == len(internal.BuildContexts) {
		return fmt.Sprintf("%s#%s", pkgURLPath, name)
	}

	// When a symbol is introduced for a specific GOOS/GOARCH at a version,
	// linking to an unspecified GOOS/GOARCH page might not take the user to
	// the symbol. Instead, link to one of the supported build contexts.
	return fmt.Sprintf("%s?GOOS=%s#%s", pkgURLPath, builds[0].GOOS, name)
}

func symbolBuilds(us *internal.UnitSymbol) []string {
	if us.InAll() {
		return nil
	}
	var builds []string
	for _, b := range us.BuildContexts() {
		builds = append(builds, b.String())
	}
	sort.Strings(builds)
	return builds
}

// createParent creates a parent symbol for the provided unit symbol. This is
// used when us is a child of a symbol that may have been introduced at a
// different version. The symbol created will have New set to false, since this
// function is only used when a parent symbol is not found for the unit symbol,
// which means it was not introduced at the same version.
func createParent(us *internal.UnitSymbol, pkgURLPath string) *Symbol {
	builds := symbolBuilds(us)
	s := &Symbol{
		Name:     us.ParentName,
		Synopsis: fmt.Sprintf("type %s", us.ParentName),
		Link:     symbolLink(pkgURLPath, us.ParentName, us.BuildContexts()),
		Section:  internal.SymbolSectionTypes,
		Kind:     internal.SymbolKindType,
		Builds:   builds,
	}
	return s
}

// sortSymbols returns an array of symbols in order of
// (1) Constants (2) Variables (3) Functions and (4) Types.
// Within each section, symbols are sorted alphabetically by name.
// In the types sections, aside from interfaces, child symbols are sorted in
// order of (1) Fields (2) Constants (3) Variables (4) Functions and (5)
// Methods. For interfaces, child symbols are sorted in order of
// (1) Methods (2) Constants (3) Variables and (4) Functions.
func sortSymbols(nameToSymbol map[string]*Symbol) [][]*Symbol {
	sm := map[internal.SymbolSection][]*Symbol{}
	for _, parent := range nameToSymbol {
		sm[parent.Section] = append(sm[parent.Section], parent)

		cm := map[internal.SymbolKind][]*Symbol{}
		parent.Synopsis = strings.TrimSuffix(parent.Synopsis, "{ ... }")
		for _, c := range parent.Children {
			cm[c.Kind] = append(cm[c.Kind], c)
		}
		for _, syms := range cm {
			sortSymbolsGroup(syms)
		}
		symbols := append(append(append(
			cm[internal.SymbolKindField],
			cm[internal.SymbolKindConstant]...),
			cm[internal.SymbolKindVariable]...),
			cm[internal.SymbolKindFunction]...)
		if strings.Contains(parent.Synopsis, "interface") {
			parent.Children = append(cm[internal.SymbolKindMethod], symbols...)
		} else {
			parent.Children = append(symbols, cm[internal.SymbolKindMethod]...)
		}
	}
	for _, syms := range sm {
		sortSymbolsGroup(syms)
	}

	var out [][]*Symbol
	for _, section := range []internal.SymbolSection{
		internal.SymbolSectionConstants,
		internal.SymbolSectionVariables,
		internal.SymbolSectionFunctions,
		internal.SymbolSectionTypes} {
		if sm[section] != nil {
			out = append(out, sm[section])
		}
	}
	return out
}

func sortSymbolsGroup(syms []*Symbol) {
	sort.Slice(syms, func(i, j int) bool {
		return syms[i].Synopsis < syms[j].Synopsis
	})
	for _, s := range syms {
		sort.Strings(s.Builds)
	}
}

// ParseVersionsDetails returns a map of versionToNameToUnitSymbol based on
// data from the proovided VersionDetails.
func ParseVersionsDetails(vd VersionsDetails) (map[string]map[string]*internal.UnitSymbol, error) {
	versionToNameToSymbol := map[string]map[string]*internal.UnitSymbol{}
	for _, vl := range vd.ThisModule {
		for _, vs := range vl.Versions {
			v := vs.Version
			versionToNameToSymbol[v] = map[string]*internal.UnitSymbol{}
			for _, syms := range vs.Symbols {
				for _, s := range syms {
					if s.New {
						versionToNameToSymbol[v][s.Name] = unitSymbol(s)
					}
					for _, c := range s.Children {
						versionToNameToSymbol[v][c.Name] = unitSymbol(c)
					}
				}
			}
		}
	}
	return versionToNameToSymbol, nil
}

// unitSymbol returns an *internal.unitSymbol from the provided *Symbol.
func unitSymbol(s *Symbol) *internal.UnitSymbol {
	us := &internal.UnitSymbol{Name: s.Name}
	if len(s.Builds) == 0 {
		us.AddBuildContext(internal.BuildContextAll)
	}
	for _, b := range s.Builds {
		parts := strings.SplitN(b, "/", 2)
		var build internal.BuildContext
		switch parts[0] {
		case "linux":
			build = internal.BuildContextLinux
		case "darwin":
			build = internal.BuildContextDarwin
		case "windows":
			build = internal.BuildContextWindows
		case "js":
			build = internal.BuildContextJS
		}
		if us.SupportsBuild(build) {
			fmt.Printf("duplicate build context for %q: %v\n", s.Name, build)
		}
		us.AddBuildContext(build)
	}
	return us
}
