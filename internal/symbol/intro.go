// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
	"sort"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// IntroducedHistory returns a map of the first version when a symbol name is
// added to the API, to the symbol name, to the UnitSymbol struct. The
// UnitSymbol.Children field will always be empty, as children names are also
// tracked.
func IntroducedHistory(sh *internal.SymbolHistory) (outSH *internal.SymbolHistory, err error) {
	defer derrors.Wrap(&err, "IntroducedHistory")
	// Generate a map of the symbol names for each build context, and the first
	// version when that symbol name was found in the package.
	buildToNameToVersion := map[internal.BuildContext]map[string]string{}
	for _, v := range sh.Versions() {
		sv := sh.SymbolsAtVersion(v)
		for _, su := range sv {
			for sm, us := range su {
				for _, build := range us.BuildContexts() {
					if _, ok := buildToNameToVersion[build]; !ok {
						buildToNameToVersion[build] = map[string]string{}
					}
					if _, ok := buildToNameToVersion[build][sm.Name]; !ok {
						buildToNameToVersion[build][sm.Name] = v
					}
				}
			}
		}
	}

	// Using the map of buildToNameToVersion, construct a symbol history,
	// where version is the first version when the symbol name was found in the
	// package.
	outSH = internal.NewSymbolHistory()
	for build, nameToVersion := range buildToNameToVersion {
		for name, version := range nameToVersion {
			us, err := sh.GetSymbol(name, version, build)
			if err != nil {
				return nil, err
			}
			outSH.AddSymbol(us.SymbolMeta, version, build)
		}
	}
	return outSH, nil
}

// LegacyIntroducedHistory returns a map of the first version when a symbol name is
// added to the API, to the symbol name, to the UnitSymbol struct. The
// UnitSymbol.Children field will always be empty, as children names are also
// tracked.
func LegacyIntroducedHistory(versionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) (
	outVersionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) {
	// Create an array of the versions in versionToNameToUnitSymbol, sorted by
	// increasing semver.
	var orderdVersions []string
	for v := range versionToNameToUnitSymbol {
		orderdVersions = append(orderdVersions, v)
	}
	sort.Slice(orderdVersions, func(i, j int) bool {
		return semver.Compare(orderdVersions[i], orderdVersions[j]) == -1
	})

	// Generate a map of the symbol names for each build context, and the first
	// version when that symbol name was found in the package.
	buildToNameToVersion := map[internal.BuildContext]map[string]string{}
	for _, v := range orderdVersions {
		nameToUnitSymbol := versionToNameToUnitSymbol[v]
		for _, us := range nameToUnitSymbol {
			for _, build := range us.BuildContexts() {
				if _, ok := buildToNameToVersion[build]; !ok {
					buildToNameToVersion[build] = map[string]string{}
				}
				if _, ok := buildToNameToVersion[build][us.Name]; !ok {
					buildToNameToVersion[build][us.Name] = v
				}
			}
		}
	}

	// Using the map of buildToNameToVersion, construct a map of
	// versionToNameToUnitSymbol, where version is the first version when the
	// symbol name was found in the package.
	outVersionToNameToUnitSymbol = map[string]map[string]*internal.UnitSymbol{}
	for build, nameToVersion := range buildToNameToVersion {
		for name, version := range nameToVersion {
			if _, ok := outVersionToNameToUnitSymbol[version]; !ok {
				outVersionToNameToUnitSymbol[version] = map[string]*internal.UnitSymbol{}
			}
			us, ok := outVersionToNameToUnitSymbol[version][name]
			if !ok {
				us = versionToNameToUnitSymbol[version][name]
				us.RemoveBuildContexts()
				outVersionToNameToUnitSymbol[version][name] = us
			}
			us.AddBuildContext(build)
		}
	}
	return outVersionToNameToUnitSymbol
}
