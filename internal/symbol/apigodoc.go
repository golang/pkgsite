// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// This file caches information about which standard library types, methods,
// and functions appeared in what version of Go
//
// Copied from
// https://go.googlesource.com/tools/+/5ab06b02d60653d5a5220fb2d99064055da3bdbd/godoc/versions.go
// with these modifications.

package symbol

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/stdlib"
)

// ParseAPIInfo parses apiVersions using contents of the specified directory.
func ParsePackageAPIInfo(files []string) (apiVersions, error) {
	// Process files in reverse semver order (vx.y.z, vz.y.z-1, ...).
	//
	// The signature of an identifier may change
	// (for example, a function that accepts a type replaced with
	// an alias), and so an existing symbol may show up again in
	// a later api/vX.Y.Z.txt file. Parsing in reverse version
	// order means we end up with the earliest version of Go
	// when the symbol was added. See golang.org/issue/44081.
	//
	ver := func(name string) string {
		base := filepath.Base(name)
		v := strings.TrimSuffix(base, ".txt")
		if strings.HasPrefix(base, "go") {
			// stdlib files have the structure goN.txt.
			// Get the semantic version.
			v = stdlib.VersionForTag(v)
		}
		return v
	}
	sort.Slice(files, func(i, j int) bool {
		return semver.Compare(ver(files[i]), ver(files[j])) > 0
	})

	vp := new(versionParser)
	for _, f := range files {
		if err := vp.parseFile(f); err != nil {
			return nil, err
		}
	}
	if len(vp.res) == 0 {
		return nil, fmt.Errorf("apiVersions should not be empty")
	}
	return vp.res, nil
}

// LoadAPIFiles loads data about the API for the given package from dir.
func LoadAPIFiles(pkgPath, dir string) ([]string, error) {
	var apiGlob string
	if stdlib.Contains(pkgPath) {
		apiGlob = filepath.Join(filepath.Clean(runtime.GOROOT()), "api", "go*.txt")
	} else {
		apiGlob = filepath.Join(dir, pkgPath, "v*.txt")
	}

	files, err := filepath.Glob(apiGlob)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files matching %q", apiGlob)
	}
	return files, nil
}

// apiVersions is a map of packages to information about those packages'
// symbols and when they were added to Go.
//
// Only things added after Go1 are tracked. Version strings are of the
// form "1.1", "1.2", etc.
type apiVersions map[string]pkgAPIVersions // keyed by Go package ("net/http")

// pkgAPIVersions contains information about which version of Go added
// certain package symbols.
//
// Only things added after Go1 are tracked. Version strings are of the
// form "1.1", "1.2", etc.
type pkgAPIVersions struct {
	constSince  map[string]string
	varSince    map[string]string
	typeSince   map[string]string            // "Server" -> "1.7"
	methodSince map[string]map[string]string // "*Server" ->"Shutdown"->1.8
	funcSince   map[string]string            // "NewServer" -> "1.7"
	fieldSince  map[string]map[string]string // "ClientTrace" -> "Got1xxResponse" -> "1.11"
}

// versionedRow represents an API feature, a parsed line of a
// $GOROOT/api/go.*txt file.
type versionedRow struct {
	pkg        string // "net/http"
	kind       string // "type", "func", "method", "field" TODO: "const", "var"
	recv       string // for methods, the receiver type ("Server", "*Server")
	name       string // name of type, (struct) field, func, method
	structName string // for struct fields, the outer struct name
}

// versionParser parses $GOROOT/api/go*.txt files and stores them in in its rows field.
type versionParser struct {
	res apiVersions // initialized lazily
}

// parseFile parses the named <apidata>/VERSION.txt file.
//
// For each row, it updates the corresponding entry in
// vp.res to VERSION, overwriting any previous value.
func (vp *versionParser) parseFile(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	base := filepath.Base(name)
	ver := strings.TrimSuffix(base, ".txt")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		row, ok := parseRow(sc.Text())
		if !ok {
			continue
		}
		if vp.res == nil {
			vp.res = make(apiVersions)
		}
		pkgi, ok := vp.res[row.pkg]
		if !ok {
			pkgi = pkgAPIVersions{
				constSince:  make(map[string]string),
				varSince:    make(map[string]string),
				typeSince:   make(map[string]string),
				methodSince: make(map[string]map[string]string),
				funcSince:   make(map[string]string),
				fieldSince:  make(map[string]map[string]string),
			}
			vp.res[row.pkg] = pkgi
		}
		switch row.kind {
		case "const":
			pkgi.constSince[row.name] = ver
		case "var":
			pkgi.varSince[row.name] = ver
		case "func":
			pkgi.funcSince[row.name] = ver
		case "type":
			pkgi.typeSince[row.name] = ver
		case "method":
			if _, ok := pkgi.methodSince[row.recv]; !ok {
				pkgi.methodSince[row.recv] = make(map[string]string)
			}
			pkgi.methodSince[row.recv][row.name] = ver
		case "field":
			if _, ok := pkgi.fieldSince[row.structName]; !ok {
				pkgi.fieldSince[row.structName] = make(map[string]string)
			}
			pkgi.fieldSince[row.structName][row.name] = ver
		}
	}
	return sc.Err()
}
func parseRow(s string) (vr versionedRow, ok bool) {
	if !strings.HasPrefix(s, "pkg ") {
		// Skip comments, blank lines, etc.
		return
	}
	rest := s[len("pkg "):]
	endPkg := strings.IndexFunc(rest, func(r rune) bool {
		return !(unicode.IsLetter(r) || r == '.' || r == '/' || r == '-' || unicode.IsDigit(r))
	})
	if endPkg == -1 {
		return
	}
	vr.pkg, rest = rest[:endPkg], rest[endPkg:]
	if !strings.HasPrefix(rest, ", ") {
		// If the part after the pkg name isn't ", ", then it's a OS/ARCH-dependent line of the form:
		//   pkg syscall (darwin-amd64), const ImplementsGetwd = false
		// We skip those for now.
		return
	}
	rest = rest[len(", "):]
	switch {
	case strings.HasPrefix(rest, "type "):
		rest = rest[len("type "):]
		sp := strings.IndexByte(rest, ' ')
		if sp == -1 {
			return
		}
		vr.name, rest = rest[:sp], rest[sp+1:]
		switch {
		case strings.HasPrefix(rest, "struct, "):
			rest = rest[len("struct, "):]
			if i := strings.IndexByte(rest, ' '); i != -1 {
				vr.kind = "field"
				vr.structName = vr.name
				vr.name = rest[:i]
				return vr, true
			}
		case strings.HasPrefix(rest, "interface, "):
			rest = rest[len("interface, "):]
			if i := strings.IndexByte(rest, '('); i != -1 {
				vr.kind = "method"
				vr.recv = vr.name
				vr.name = rest[:i]
				return vr, true
			}
		default:
			vr.kind = "type"
			return vr, true
		}
	case strings.HasPrefix(rest, "const "):
		vr.kind = "const"
		rest = rest[len("const "):]
		if i := strings.IndexByte(rest, ' '); i != -1 {
			vr.name = rest[:i]
			return vr, true
		}
	case strings.HasPrefix(rest, "var "):
		vr.kind = "var"
		rest = rest[len("var "):]
		if i := strings.IndexByte(rest, ' '); i != -1 {
			vr.name = rest[:i]
			return vr, true
		}
	case strings.HasPrefix(rest, "func "):
		vr.kind = "func"
		rest = rest[len("func "):]
		if i := strings.IndexByte(rest, '('); i != -1 {
			vr.name = rest[:i]
			return vr, true
		}
	case strings.HasPrefix(rest, "method "): // "method (*File) SetModTime(time.Time)"
		vr.kind = "method"
		rest = rest[len("method "):] // "(*File) SetModTime(time.Time)"
		sp := strings.IndexByte(rest, ' ')
		if sp == -1 {
			return
		}
		vr.recv = strings.Trim(rest[:sp], "()")    // "*File"
		vr.recv = strings.TrimPrefix(vr.recv, "*") // "File"
		rest = rest[sp+1:]                         // SetMode(os.FileMode)
		paren := strings.IndexByte(rest, '(')
		if paren == -1 {
			return
		}
		vr.name = rest[:paren]
		return vr, true
	}
	return // TODO: handle more cases
}
