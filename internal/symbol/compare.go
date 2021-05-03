// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// CompareAPIVersions returns the differences between apiVersions and
// inVersionToNameToUnitSymbol.
func CompareAPIVersions(path string, apiVersions pkgAPIVersions,
	inVersionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) []string {
	versionToNameToUnitSymbol := LegacyIntroducedHistory(inVersionToNameToUnitSymbol)

	// Create a map of name to the first version when the symbol name was found
	// in the package.
	nameToVersion := map[string]string{}
	for version, nts := range versionToNameToUnitSymbol {
		for name := range nts {
			if _, ok := nameToVersion[name]; !ok {
				nameToVersion[name] = version
				continue
			}
			// Track the first version when the symbol name is added. It is
			// possible for the symbol name to appear in multiple versions if
			// it is introduced at different build contexts. The godoc
			// logic that generates apiVersions does not take build
			// context info into account.
			if semver.Compare(version, nameToVersion[name]) == -1 {
				nameToVersion[name] = version
			}
		}
	}

	var errors []string

	shouldSkip := func(name string) bool {
		if strings.HasSuffix(name, "embedded") {
			// The Go api/goN.txt files contain a Foo.embedded row when a new
			// field is added. pkgsite does not currently handle embedding, so
			// skip this check.
			//
			// type UnspecifiedType struct, embedded BasicType in
			// https://go.googlesource.com/go/+/0e85fd7561de869add933801c531bf25dee9561c/api/go1.4.txt#62
			// is an example.
			//
			// cmd/api code at
			// https://go.googlesource.com/go/+/go1.16/src/cmd/api/goapi.go#924.
			return true
		}
		if methods, ok := pathToEmbeddedMethods[path]; ok {
			if _, ok := methods[name]; ok {
				return true
			}
		}
		if exceptions, ok := pathToExceptions[path]; ok {
			if _, ok := exceptions[name]; ok {
				return true
			}
		}
		return false
	}
	check := func(name, wantVersion string) {
		if stdlib.Contains(path) && shouldSkip(name) {
			return
		}

		got, ok := nameToVersion[name]
		delete(nameToVersion, name)
		if !ok {
			errors = append(errors, fmt.Sprintf("not found: (want %q) %q \n", wantVersion, name))
		} else if got != wantVersion {
			errors = append(errors, fmt.Sprintf("mismatch: (want %q | got %q) %q\n", wantVersion, got, name))
		}
	}

	for _, m := range []map[string]string{
		apiVersions.constSince,
		apiVersions.varSince,
		apiVersions.funcSince,
		apiVersions.typeSince,
	} {
		for name, version := range m {
			check(name, version)
		}
	}
	for typ, method := range apiVersions.methodSince {
		for name, version := range method {
			typ = strings.TrimPrefix(typ, "*")
			check(typ+"."+name, version)
		}
	}
	for typ, field := range apiVersions.fieldSince {
		for name, version := range field {
			check(typ+"."+name, version)
		}
	}
	for name, version := range nameToVersion {
		errors = append(errors, fmt.Sprintf("extra symbol: %q %q\n", name, version))
	}
	sort.Strings(errors)
	return errors
}
