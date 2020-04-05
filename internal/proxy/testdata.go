// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

var defaultModules = []*TestModule{
	{
		ModulePath: "build.constraints/module",
		Files: map[string]string{
			"LICENSE": LicenseBSD3,
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
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"go.mod":      "module github.com/my/module\n\ngo 1.12",
			"LICENSE":     LicenseBSD3,
			"README.md":   "README FILE FOR TESTING.",
			"bar/LICENSE": LicenseMIT,
			"bar/bar.go": `
				// package bar
				package bar

				// Bar returns the string "bar".
				func Bar() string {
					return "bar"
				}`,
			"foo/LICENSE.md": LicenseMIT,
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
			"go.mod":          "module nonredistributable.mod/module\n\ngo 1.13",
			"LICENSE":         LicenseBSD3,
			"README.md":       "README FILE FOR TESTING.",
			"bar/baz/COPYING": LicenseMIT,
			"bar/baz/baz.go": `
				// package baz
				package baz

				// Baz returns the string "baz".
				func Baz() string {
					return "baz"
				}
				`,
			"bar/LICENSE": LicenseMIT,
			"bar/bar.go": `
				// package bar
				package bar

				// Bar returns the string "bar".
				func Bar() string {
					return "bar"
				}`,
			"foo/LICENSE.md": LicenseCCNC,
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
