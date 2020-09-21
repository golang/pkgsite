// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func TestMatchingFiles(t *testing.T) {
	plainGoBody := `
		package plain
		type Value int`
	jsGoBody := `
		// +build js,wasm

		// Package js only works with wasm.
		package js
		type Value int`

	plainContents := map[string]string{
		"README.md":      "THIS IS A README",
		"LICENSE.md":     testhelper.MITLicense,
		"plain/plain.go": plainGoBody,
	}

	jsContents := map[string]string{
		"README.md":  "THIS IS A README",
		"LICENSE.md": testhelper.MITLicense,
		"js/js.go":   jsGoBody,
	}
	for _, test := range []struct {
		name         string
		goos, goarch string
		contents     map[string]string
		want         map[string][]byte
	}{
		{
			name:     "plain-linux",
			goos:     "linux",
			goarch:   "amd64",
			contents: plainContents,
			want: map[string][]byte{
				"plain.go": []byte(plainGoBody),
			},
		},
		{
			name:     "plain-js",
			goos:     "js",
			goarch:   "wasm",
			contents: plainContents,
			want: map[string][]byte{
				"plain.go": []byte(plainGoBody),
			},
		},
		{
			name:     "wasm-linux",
			goos:     "linux",
			goarch:   "amd64",
			contents: jsContents,
			want:     map[string][]byte{},
		},
		{
			name:     "wasm-js",
			goos:     "js",
			goarch:   "wasm",
			contents: jsContents,
			want: map[string][]byte{
				"js.go": []byte(jsGoBody),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := testhelper.ZipContents(test.contents)
			if err != nil {
				t.Fatal(err)
			}
			r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := matchingFiles(test.goos, test.goarch, r.File)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRemoveUnusedASTNodes(t *testing.T) {
	const file = `
// Package-level comment.
package p

// const C
const C = 1

// leave unexported consts
const c = 1

// var V
var V int

// leave unexported vars
var v int

// type T
type T int

// leave unexported types
type t int

// Exp is exported.
func Exp() {}

// unexp is not exported.
func unexp() {}

// M is exported.
func (t T) M() int {}

// m isn't.
func (T) m() {}

// U is an exported method of an unexported type.
// Its doc is not shown, unless t is embedded
// in an exported type. We don't remove it to
// be safe.
func (t) U() {}
`
	////////////////
	const want = `// Package-level comment.
package p

// const C
const C = 1

// leave unexported consts
const c = 1

// var V
var V int

// leave unexported vars
var v int

// type T
type T int

// leave unexported types
type t int

// Exp is exported.
func Exp()

// M is exported.
func (t T) M() int

// U is an exported method of an unexported type.
// Its doc is not shown, unless t is embedded
// in an exported type. We don't remove it to
// be safe.
func (t) U()
`
	////////////////

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "tst.go", file, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	removeUnusedASTNodes(astFile)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, astFile); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
