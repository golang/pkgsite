// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// Symbol is an element in the package API. A symbol can be a constant,
// variable, function, type, field or method.
type Symbol struct {
	// Name is name of the symbol. At a given package version, name must be
	// unique.
	Name string

	// Synopsis is the one line description of the symbol that is displayed.
	Synopsis string

	// Section is the section that a symbol appears in.
	Section internal.SymbolSection

	// Kind is the type of a symbol, which is either a constant, variable,
	// function, type, field or method.
	Kind internal.SymbolKind

	// Link is the link to the symbol name on pkg.go.dev.
	Link string

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

	// builds keeps track of build contexts used to generate Builds.
	builds map[internal.BuildContext]bool

	// New indicates that the symbol is new as of the version where it is
	// present. For example, if type Client was introduced in v1.0.0 and
	// Client.Timeout was introduced in v1.1.0, New will be false for Client
	// and true for Client.Timeout if this Symbol corresponds to v1.1.0.
	New bool
}

func (s *Symbol) addBuilds(builds ...internal.BuildContext) {
	if s.builds == nil {
		s.builds = map[internal.BuildContext]bool{}
	}
	for _, b := range builds {
		s.builds[b] = true
	}
}

// symbolsForVersions returns an array of symbols for use in the VersionSummary
// of the specified version.
func symbolsForVersion(pkgURLPath string, symbolsAtVersion map[string]map[internal.SymbolMeta]*internal.SymbolBuildContexts) [][]*Symbol {
	nameToMetaToSymbol := map[string]map[internal.SymbolMeta]*Symbol{}
	children := map[internal.SymbolMeta]*internal.SymbolBuildContexts{}
	for _, smToUs := range symbolsAtVersion {
		for sm, us := range smToUs {
			if sm.ParentName != sm.Name {
				// For the children, keep track of them for later.
				children[sm] = us
				continue
			}

			metaToSym, ok := nameToMetaToSymbol[sm.Name]
			if !ok {
				metaToSym = map[internal.SymbolMeta]*Symbol{}
				nameToMetaToSymbol[sm.Name] = metaToSym
			}
			s, ok := metaToSym[sm]
			if !ok {
				s = &Symbol{
					Name:     sm.Name,
					Synopsis: sm.Synopsis,
					Section:  sm.Section,
					Kind:     sm.Kind,
					Link:     symbolLink(pkgURLPath, sm.Name, us.BuildContexts()),
					New:      true,
				}
				nameToMetaToSymbol[s.Name][sm] = s
			}
			s.addBuilds(us.BuildContexts()...)
		}
	}

	for cm, cus := range children {
		// For each child symbol, 1 of 3 things can occur:
		//
		// Option 1: If no parent exists for this child symbol, make one
		// and add the parent to the map.
		//
		// Option 2: A parent exists and does not support the build context
		// of the child. This occurs when the parent type is introduced for
		// another build context, but was introduced at the previous version
		// for the current child. Create a new parent for this child.
		//
		// Option 3: A parent exists and does support the build context of
		// the child. Add the child to the parent.
		cs := &Symbol{
			Name:     cm.Name,
			Synopsis: cm.Synopsis,
			Section:  cm.Section,
			Kind:     cm.Kind,
			Link:     symbolLink(pkgURLPath, cm.Name, cus.BuildContexts()),
			New:      true,
		}

		ps := findParent(cm.ParentName, cus, nameToMetaToSymbol)
		if ps != nil {
			// Option 3: found a relevant parent.
			ps.Children = append(ps.Children, cs)
			continue
		}

		// Option 1 and 2: We did not find a relevant parent, so create
		// one.
		//
		// Since this parent is not introduced at this version, create
		// a distinct type for each group of symbols.
		// To do so, we make up a synopsis for the SymbolMeta below, since it
		// is only used as a key in nameToMetaToSymbol.
		//
		// Example case:
		// http://pkg.go.dev/internal/poll?tab=versions for go1.10 should show:
		//
		// type FD -- windows/amd64
		//		FD.ReadMsg
		//		FD.WriteMsg
		// type FD -- darwin/amd64, linux/amd64
		//		FD.SetBlocking
		//		FD.WriteOnce
		ps = createParent(cm.ParentName, pkgURLPath, cus.BuildContexts()...)
		pm := internal.SymbolMeta{
			Name:       ps.Name,
			ParentName: ps.Name,
			Synopsis:   fmt.Sprintf("type %s (%v)", ps.Name, cus.BuildContexts()),
			Section:    ps.Section,
			Kind:       ps.Kind,
		}
		ps.Children = append(ps.Children, cs)
		if _, ok := nameToMetaToSymbol[pm.Name]; !ok {
			nameToMetaToSymbol[pm.Name] = map[internal.SymbolMeta]*Symbol{}
		}
		nameToMetaToSymbol[pm.Name][pm] = ps
	}

	var symbols []*Symbol
	for _, mts := range nameToMetaToSymbol {
		for _, s := range mts {
			if len(s.builds) != len(internal.BuildContexts) {
				for b := range s.builds {
					s.Builds = append(s.Builds, fmt.Sprintf("%s/%s", b.GOOS, b.GOARCH))
				}
				sort.Strings(s.Builds)
			}
			symbols = append(symbols, s)
		}
	}
	return sortSymbols(symbols)
}

func findParent(parentName string, cus *internal.SymbolBuildContexts,
	nameToMetaToSymbol map[string]map[internal.SymbolMeta]*Symbol) *Symbol {
	parents, ok := nameToMetaToSymbol[parentName]
	if !ok {
		return nil
	}
	for _, ps := range parents {
		for build := range ps.builds {
			if cus.SupportsBuild(build) {
				return ps
			}
		}
	}
	return nil
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

// createParent creates a parent symbol for the provided unit symbol. This is
// used when us is a child of a symbol that may have been introduced at a
// different version. The symbol created will have New set to false, since this
// function is only used when a parent symbol is not found for the unit symbol,
// which means it was not introduced at the same version.
func createParent(parentName, pkgURLPath string, builds ...internal.BuildContext) *Symbol {
	s := &Symbol{
		Name:     parentName,
		Synopsis: fmt.Sprintf("type %s", parentName),
		Section:  internal.SymbolSectionTypes,
		Kind:     internal.SymbolKindType,
		Link:     symbolLink(pkgURLPath, parentName, builds),
	}
	s.addBuilds(builds...)
	return s
}

// sortSymbols returns an array of symbols in order of
// (1) Constants (2) Variables (3) Functions and (4) Types.
// Within each section, symbols are sorted alphabetically by name.
// In the types sections, aside from interfaces, child symbols are sorted in
// order of (1) Fields (2) Constants (3) Variables (4) Functions and (5)
// Methods. For interfaces, child symbols are sorted in order of
// (1) Methods (2) Constants (3) Variables and (4) Functions.
func sortSymbols(symbols []*Symbol) [][]*Symbol {
	sm := map[internal.SymbolSection][]*Symbol{}
	for _, parent := range symbols {
		sm[parent.Section] = append(sm[parent.Section], parent)
		cm := map[internal.SymbolKind][]*Symbol{}
		parent.Synopsis = strings.TrimSuffix(strings.TrimSuffix(parent.Synopsis, "{ ... }"), "{}")
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
		s1 := syms[i]
		s2 := syms[j]
		if s1.Synopsis != s2.Synopsis {
			return s1.Synopsis < s2.Synopsis
		}
		return compareStringSlices(s1.Builds, s2.Builds) < 0
	})
}

func compareStringSlices(ss1, ss2 []string) int {
	for i, s1 := range ss1 {
		if i >= len(ss2) { // first slice is longer, so greater
			return 1
		}
		if c := strings.Compare(s1, ss2[i]); c != 0 {
			return c
		}
	}
	if len(ss1) == len(ss2) {
		return 0
	}
	// first slice is shorter
	return -1
}

// ParseVersionsDetails returns a map of versionToNameToUnitSymbol based on
// data from the proovided VersionDetails.
func ParseVersionsDetails(vd *VersionsDetails) (_ *internal.SymbolHistory, err error) {
	sh := internal.NewSymbolHistory()
	for _, vl := range vd.ThisModule {
		for _, vs := range vl.Versions {
			v := vs.Version
			if vd.ThisModule[0].ModulePath == stdlib.ModulePath {
				v = stdlib.VersionForTag(v)
			}
			for _, syms := range vs.Symbols {
				for _, s := range syms {
					if s.New {
						addSymbol(s, v, sh, s.Builds)
					}
					for _, c := range s.Children {
						addSymbol(c, v, sh, s.Builds)
					}
				}
			}
		}
	}
	return sh, nil
}

func addSymbol(s *Symbol, v string, sh *internal.SymbolHistory, builds []string) {
	sm := internal.SymbolMeta{
		Name: s.Name,
	}
	if len(builds) == 0 {
		sh.AddSymbol(sm, v, internal.BuildContextAll)
		return
	}
	for _, b := range builds {
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
		sh.AddSymbol(sm, v, build)
	}
}
