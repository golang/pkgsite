// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
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
			sm, err := sh.GetSymbol(name, version, build)
			if err != nil {
				return nil, err
			}
			outSH.AddSymbol(*sm, version, build)
		}
	}
	return outSH, nil
}
