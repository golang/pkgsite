// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
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

func TestMergePackages(t *testing.T) {
	doc1 := &internal.Documentation{
		GOOS:     "linux",
		GOARCH:   "amd64",
		Synopsis: "s1",
	}
	doc2 := &internal.Documentation{
		GOOS:     "js",
		GOARCH:   "wasm",
		Synopsis: "s2",
	}

	pkg := func(name, imp string, doc *internal.Documentation, err error) *goPackage {
		return &goPackage{
			name:    name,
			imports: []string{imp},
			docs:    []*internal.Documentation{doc},
			err:     err,
		}
	}

	for _, test := range []struct {
		name    string
		pkgs    []*goPackage
		want    *goPackage
		wantErr bool
	}{
		{
			name:    "no packages",
			pkgs:    nil,
			want:    nil,
			wantErr: false,
		},
		{
			name: "one package",
			pkgs: []*goPackage{pkg("name1", "imp1", doc1, nil)},
			want: pkg("name1", "imp1", doc1, nil),
		},
		{
			name: "two packages",
			pkgs: []*goPackage{
				pkg("name1", "imp1", doc1, nil),
				pkg("name1", "imp2", doc2, nil),
			},
			want: &goPackage{
				name:    "name1",
				imports: []string{"imp1"},                      // keep the first one
				docs:    []*internal.Documentation{doc1, doc2}, // keep both, in order
			},
		},
		{
			name: "one package has err",
			pkgs: []*goPackage{
				pkg("name1", "imp1", doc1, nil),
				pkg("name1", "imp2", doc2, godoc.ErrTooLarge),
			},
			want: pkg("name1", "imp2", doc2, godoc.ErrTooLarge), // return one with err
		},
		{
			name: "different names",
			pkgs: []*goPackage{
				pkg("name1", "imp1", doc1, nil),
				pkg("name2", "imp2", doc2, nil),
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := mergePackages(test.pkgs)
			if err != nil {
				if !test.wantErr {
					t.Fatalf("got %v, want no error", err)
				}
				return
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(goPackage{}), cmpopts.EquateErrors()); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
