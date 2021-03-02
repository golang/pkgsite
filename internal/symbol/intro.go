// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
	"sort"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
)

// IntroducedHistory returns a map of the first version when a symbol name is
// added to the API, to the symbol name, to the UnitSymbol struct. The
// UnitSymbol.Children field will always be empty, as children names are also
// tracked.
func IntroducedHistory(versionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) (
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
	for _, nameToVersion := range buildToNameToVersion {
		for name, version := range nameToVersion {
			if _, ok := outVersionToNameToUnitSymbol[version]; !ok {
				outVersionToNameToUnitSymbol[version] = map[string]*internal.UnitSymbol{}
			}
			us := versionToNameToUnitSymbol[version][name]
			outVersionToNameToUnitSymbol[version][name] = us
		}
	}
	return outVersionToNameToUnitSymbol
}
