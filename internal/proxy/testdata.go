// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

var testdata = []*TestModule{
	{
		ModulePath: "bad.mod/module",
		Files: map[string]string{
			"LICENSE": licenseBSD3,
			"good/good.go": `
			// Package good is inside a module that has bad packages.
			package good

			// Good is whether this package is good.
			const Good = true`,

			"illegalchar/p.go": `
			package p

			func init() {
				var c00 uint8 = '\0';  // ERROR "oct|char"
				var c01 uint8 = '\07';  // ERROR "oct|char"
				var cx0 uint8 = '\x0';  // ERROR "hex|char"
				var cx1 uint8 = '\x';  // ERROR "hex|char"
				_, _, _, _ = c00, c01, cx0, cx1
			}
			`,
			"multiplepkgs/a.go": "package a",
			"multiplepkgs/b.go": "package b",
		},
	},
	{
		ModulePath: "emp.ty/module",
	},
	{
		ModulePath: "emp.ty/package",
		Files: map[string]string{
			"main.go": "package main",
		},
	},
	{
		ModulePath:   "build.constraints/module",
		ExcludeGoMod: true,
		Files: map[string]string{
			"LICENSE": licenseBSD3,
			"cpu/cpu.go": `
				// Package cpu implements processor feature detection
				// used by the Go standard library.
				package cpu`,
			"cpu/cpu_arm.go":   "package cpu\n\nconst CacheLinePadSize = 1",
			"cpu/cpu_arm64.go": "package cpu\n\nconst CacheLinePadSize = 2",
			"cpu/cpu_x86.go":   "// +build 386 amd64 amd64p32\n\npackage cpu\n\nconst CacheLinePadSize = 3",
			"ignore/ignore.go": "// +build ignore\n\npackage ignore",
		},
	},
	{
		ModulePath: "no.mod/module",
		Files: map[string]string{
			"LICENSE": licenseBSD3,
			"p/p.go": `
				// Package p is inside a module where a go.mod
				// file hasn't been explicitly added yet.
				package p

				// Year is a year before go.mod files existed.
				const Year = 2009`,
		},
	},
	{
		ModulePath: "doc.test",
		Files: map[string]string{
			"LICENSE": licenseBSD3,
			"permalink/doc.go": `
				// Package permalink is for testing the heading
				// permalink documentation rendering feature.
				//
				// This is a heading
				//
				// This is a paragraph.
				//
				// This is yet another
				// paragraph.
				//
				package permalink`,
		},
	},
	{
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"LICENSE":     licenseBSD3,
			"README.md":   "README FILE FOR TESTING.",
			"bar/LICENSE": licenseMIT,
			"bar/bar.go": `
				// package bar
				package bar

				// Bar returns the string "bar".
				func Bar() string {
					return "bar"
				}`,
			"foo/LICENSE.md": licenseMIT,
			"foo/foo.go": `
				// package foo
				package foo

				import (
					"fmt"

					"github.com/my/module/bar"
				)

				// FooBar returns the string "foo bar".
				func FooBar() string {
					return fmt.Sprintf("foo %s", bar.Bar())
				}`,
		},
	},
	{
		ModulePath: "nonredistributable.mod/module",
		Files: map[string]string{
			"LICENSE":         licenseBSD3,
			"README.md":       "README FILE FOR TESTING.",
			"bar/baz/COPYING": licenseMIT,
			"bar/baz/baz.go": `
				// package baz
				package baz

				// Baz returns the string "baz".
				func Baz() string {
					return "baz"
				}
				`,
			"bar/LICENSE": licenseMIT,
			"bar/bar.go": `
				// package bar
				package bar

				// Bar returns the string "bar".
				func Bar() string {
					return "bar"
				}`,
			"foo/LICENSE.md": licenseCCNC,
			"foo/foo.go": `
				// package foo
				package foo

				import (
					"fmt"

					"github.com/my/module/bar"
				)

				// FooBar returns the string "foo bar".
				func FooBar() string {
					return fmt.Sprintf("foo %s", bar.Bar())
				}`,
		},
	},
}

// defaultTestModules creates testModules for the modules in the defaultTestModules*.go
// files.
func defaultTestModules() []*TestModule {
	var modules []*TestModule
	for _, m := range testdata {
		if m.Version == "" {
			m.Version = "v1.0.0"
		}
		if !m.ExcludeGoMod {
			if m.Files == nil {
				m.Files = map[string]string{}
			}
			if m.Files != nil {
				if _, ok := m.Files["go.mod"]; !ok {
					m.Files["go.mod"] = defaultGoMod(m.ModulePath)
				}
			}
		}
		modules = append(modules, m)
	}
	return modules
}
