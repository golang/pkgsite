// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doc_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
	"golang.org/x/tools/txtar"
)

func TestExamples2(t *testing.T) {
	dir := filepath.Join("testdata", "examples")
	filenames, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range filenames {
		t.Run(strings.TrimSuffix(filepath.Base(filename), ".go"), func(t *testing.T) {
			fset := token.NewFileSet()
			astFile, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			goldenFilename := strings.TrimSuffix(filename, ".go") + ".golden"
			golden, err := readSectionFile(goldenFilename)
			if err != nil {
				t.Fatal(err)
			}
			examples := map[string]*doc.Example{}
			unseen := map[string]bool{} // examples we haven't seen yet
			for _, e := range doc.Examples2(fset, astFile) {
				examples[e.Name] = e
				unseen[e.Name] = true
			}
			for section, want := range golden {
				words := strings.Split(section, ".")
				if len(words) != 2 {
					t.Fatalf("bad section name %q", section)
				}
				name, kind := words[0], words[1]
				ex := examples[name]
				if ex == nil {
					t.Fatalf("no example named %q", name)
				}
				switch kind {
				case "Play":
					got := strings.TrimSpace(formatFile(t, fset, ex.Play))
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("%s Play: mismatch (-want, +got):\n%s", name, diff)
					}
					delete(unseen, name)
				case "Output":
					got := strings.TrimSpace(ex.Output)
					if got != want {
						t.Errorf("%s Output: got\n%q\n---- want ----\n%q", ex.Name, got, want)
					}
				default:
					t.Fatalf("bad section kind %q", kind)
				}
			}
			for name := range unseen {
				t.Errorf("no Play golden for example %q", name)
			}
		})
	}
}

// readSectionFile reads a file that is divided into sections, and returns
// a map from section name to contents.
//
// We use the txtar format for the file. See https://pkg.go.dev/golang.org/x/tools/txtar.
// Although the format talks about filenames as the keys, they can be arbitrary strings.
func readSectionFile(filename string) (map[string]string, error) {
	archive, err := txtar.ParseFile(filename)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, f := range archive.Files {
		m[f.Name] = strings.TrimSpace(string(f.Data))
	}
	return m, nil
}
