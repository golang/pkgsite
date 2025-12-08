// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importer

import (
	"go/ast"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SimpleImporter returns a (dummy) package object named by the last path element,
// stripping off any major version suffix or go- prefix.
// This is sufficient to resolve most package identifiers without doing an actual
// import. It never returns an error.
//
//lint:ignore SA1019 We had a preexisting dependency on ast.Object.
func SimpleImporter(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg == nil {
		name := importPathToAssumedName(path)
		pkg = ast.NewObj(ast.Pkg, name)
		pkg.Data = ast.NewScope(nil) // required by ast.NewPackage for dot-import
		imports[path] = pkg
	}
	return pkg, nil
}

// importPathToAssumedName is a copy of golang.org/x/tools/internal/imports.ImportPathToAssumedName
func importPathToAssumedName(importPath string) string {
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			dir := path.Dir(importPath)
			if dir != "." {
				base = path.Base(dir)
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, notIdentifier); i >= 0 {
		base = base[:i]
	}
	return base
}

// notIdentifier reports whether ch is an invalid identifier character.
func notIdentifier(ch rune) bool {
	return !('a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' ||
		'0' <= ch && ch <= '9' ||
		ch == '_' ||
		ch >= utf8.RuneSelf && (unicode.IsLetter(ch) || unicode.IsDigit(ch)))
}
