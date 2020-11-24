// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"net/http"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

type testModule struct {
	mod        *proxy.Module
	fr         *FetchResult
	docStrings map[string][]string
}

var moduleOnePackage = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/basic",
		Files: map[string]string{
			"README.md":  "THIS IS A README",
			"foo/foo.go": "// package foo exports a helpful constant.\npackage foo\nimport \"net/http\"\nconst OK = http.StatusOK",
			"LICENSE":    testhelper.BSD0License,
		},
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/basic",
				HasGoMod:   false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/basic",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "THIS IS A README",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "github.com/basic/foo",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package foo exports a helpful constant.",
					},
					Imports: []string{"net/http"},
				},
			},
		},
	},
}

var moduleMultiPackage = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"go.mod":        "module github.com/my/module\n\ngo 1.12",
			"LICENSE":       testhelper.BSD0License,
			"README.md":     "README FILE FOR TESTING.",
			"bar/COPYING":   testhelper.MITLicense,
			"bar/README.md": "Another README FILE FOR TESTING.",
			"bar/bar.go": `
			// package bar
			package bar

			// Bar returns the string "bar".
			func Bar() string {
				return "bar"
			}`,
			"foo/LICENSE.md": testhelper.MITLicense,
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
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/my/module",
				HasGoMod:   true,
				SourceInfo: source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/my/module",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README FILE FOR TESTING.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "bar",
						Path: "github.com/my/module/bar",
					},
					Readme: &internal.Readme{
						Filepath: "bar/README.md",
						Contents: "Another README FILE FOR TESTING.",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package bar",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "github.com/my/module/foo",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package foo",
					},
					Imports: []string{"fmt", "github.com/my/module/bar"},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"github.com/my/module/bar": {"Bar returns the string &#34;bar&#34;."},
		"github.com/my/module/foo": {"FooBar returns the string &#34;foo bar&#34;."},
	},
}

var moduleNoGoMod = &testModule{
	mod: &proxy.Module{
		ModulePath: "no.mod/module",
		Files: map[string]string{
			"LICENSE": testhelper.BSD0License,
			"p/p.go": `
				// Package p is inside a module where a go.mod
				// file hasn't been explicitly added yet.
				package p

				// Year is a year before go.mod files existed.
				const Year = 2009`,
		},
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "no.mod/module",
				HasGoMod:   false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "no.mod/module",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "p",
						Path: "no.mod/module/p",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package p is inside a module where a go.mod file hasn't been explicitly added yet.",
					},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"no.mod/module/p": {"const Year = 2009"},
	},
}

var moduleEmpty = &testModule{
	mod: &proxy.Module{
		ModulePath: "emp.ty/module",
	},
	fr: &FetchResult{Module: &internal.Module{}},
}

var moduleBadPackages = &testModule{
	mod: &proxy.Module{
		ModulePath: "bad.mod/module",
		Files: map[string]string{
			"LICENSE": testhelper.BSD0License,
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
	fr: &FetchResult{
		Status: derrors.ToStatus(derrors.HasIncompletePackages),
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "bad.mod/module",
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "bad.mod/module",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "good",
						Path: "bad.mod/module/good",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package good is inside a module that has bad packages.",
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				PackagePath: "bad.mod/module/good",
				ModulePath:  "bad.mod/module",
				Version:     "v1.0.0",
				Status:      200,
			},
			{
				PackagePath: "bad.mod/module/illegalchar",
				ModulePath:  "bad.mod/module",
				Version:     "v1.0.0",
				Status:      600,
			},
			{
				PackagePath: "bad.mod/module/multiplepkgs",
				ModulePath:  "bad.mod/module",
				Version:     "v1.0.0",
				Status:      600,
			},
		},
	},
	docStrings: map[string][]string{
		"bad.mod/module/good": {`const Good = <a href="/builtin#true">true</a>`},
	},
}

var moduleBuildConstraints = &testModule{
	mod: &proxy.Module{
		ModulePath: "build.constraints/module",
		Files: map[string]string{
			"LICENSE": testhelper.BSD0License,
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
	fr: &FetchResult{
		Status: derrors.ToStatus(derrors.HasIncompletePackages),
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "build.constraints/module",
				HasGoMod:   false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "build.constraints/module",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "cpu",
						Path: "build.constraints/module/cpu",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				ModulePath:  "build.constraints/module",
				Version:     "v1.0.0",
				PackagePath: "build.constraints/module/cpu",
				Status:      http.StatusOK,
			},
			{
				ModulePath:  "build.constraints/module",
				Version:     "v1.0.0",
				PackagePath: "build.constraints/module/ignore",
				Status:      derrors.ToStatus(derrors.PackageBuildContextNotSupported),
			},
		},
	},
	docStrings: map[string][]string{
		"build.constraints/module/cpu": {"const CacheLinePadSize = 3"},
	},
}

var moduleNonRedist = &testModule{
	mod: &proxy.Module{
		ModulePath: "nonredistributable.mod/module",
		Files: map[string]string{
			"go.mod":          "module nonredistributable.mod/module\n\ngo 1.13",
			"LICENSE":         testhelper.BSD0License,
			"README.md":       "README FILE FOR TESTING.",
			"bar/baz/COPYING": testhelper.MITLicense,
			"bar/baz/baz.go": `
			// package baz
			package baz

			// Baz returns the string "baz".
			func Baz() string {
				return "baz"
			}
			`,
			"bar/LICENSE": testhelper.MITLicense,
			"bar/bar.go": `
			// package bar
			package bar

			// Bar returns the string "bar".
			func Bar() string {
				return "bar"
			}`,
			"foo/README.md":  "README FILE SHOW UP HERE BUT WILL BE REMOVED BEFORE DB INSERT",
			"foo/LICENSE.md": testhelper.UnknownLicense,
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
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "nonredistributable.mod/module",
				HasGoMod:   true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "nonredistributable.mod/module",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README FILE FOR TESTING.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "bar",
						Path: "nonredistributable.mod/module/bar",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package bar",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "baz",
						Path: "nonredistributable.mod/module/bar/baz",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package baz",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "nonredistributable.mod/module/foo",
					},
					Readme: &internal.Readme{
						Filepath: "foo/README.md",
						Contents: "README FILE SHOW UP HERE BUT WILL BE REMOVED BEFORE DB INSERT",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package foo",
					},
					Imports: []string{"fmt", "github.com/my/module/bar"},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"nonredistributable.mod/module/bar":     {"Bar returns the string"},
		"nonredistributable.mod/module/bar/baz": {"Baz returns the string"},
		"nonredistributable.mod/module/foo":     {"FooBar returns the string"},
	},
}

var moduleBadImportPath = &testModule{
	mod: &proxy.Module{
		ModulePath: "bad.import.path.com",
		Files: map[string]string{
			"good/import/path/foo.go": "package foo",
			"bad/import path/foo.go":  "package foo",
		},
	},
	fr: &FetchResult{
		Status: derrors.ToStatus(derrors.HasIncompletePackages),
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "bad.import.path.com",
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "bad.import.path.com",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "bad.import.path.com/good",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "bad.import.path.com/good/import",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "bad.import.path.com/good/import/path",
					},
					Documentation: &internal.Documentation{},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				ModulePath:  "bad.import.path.com",
				PackagePath: "bad.import.path.com/bad/import path",
				Version:     "v1.0.0",
				Status:      derrors.ToStatus(derrors.PackageBadImportPath),
			},
			{
				ModulePath:  "bad.import.path.com",
				PackagePath: "bad.import.path.com/good/import/path",
				Version:     "v1.0.0",
				Status:      http.StatusOK,
			},
		},
	},
}

var moduleDocTest = &testModule{
	mod: &proxy.Module{
		ModulePath: "doc.test",
		Files: map[string]string{
			"LICENSE": testhelper.BSD0License,
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
	fr: &FetchResult{
		GoModPath: "doc.test",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "doc.test",
				HasGoMod:   false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "doc.test",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "permalink",
						Path: "doc.test/permalink",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package permalink is for testing the heading permalink documentation rendering feature.",
					},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"doc.test/permalink": {
			"<h4 id=\"hdr-This_is_a_heading\">This is a heading <a",
			"href=\"#hdr-This_is_a_heading\">¶</a></h4>",
		},
	},
}

var moduleDocTooLarge = &testModule{
	mod: &proxy.Module{
		ModulePath: "bigdoc.test",
		Files: map[string]string{
			"LICENSE": testhelper.BSD0License,
			"doc.go": "// This documentation is big.\n" +
				strings.Repeat("// Too big.\n", 200_000) +
				"package bigdoc",
		},
	},
	fr: &FetchResult{
		Status:    derrors.ToStatus(derrors.HasIncompletePackages),
		GoModPath: "bigdoc.test",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "bigdoc.test",
				HasGoMod:   false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Name: "bigdoc",
						Path: "bigdoc.test",
					},
					Documentation: &internal.Documentation{
						Synopsis: "This documentation is big.",
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				PackagePath: "bigdoc.test",
				ModulePath:  "bigdoc.test",
				Version:     "v1.0.0",
				Status:      derrors.ToStatus(derrors.PackageDocumentationHTMLTooLarge),
			},
		},
	},
	docStrings: map[string][]string{
		"bigdoc.test": {godoc.DocTooLargeReplacement},
	},
}

var moduleWasm = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/my/module/js",
		Files: map[string]string{

			"README.md": "THIS IS A README",
			"LICENSE":   testhelper.BSD0License,
			"js/js.go": `
					// +build js,wasm

					// Package js only works with wasm.
					package js
					type Value int`,
		},
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/my/module/js",
				SourceInfo: source.NewGitHubInfo("https://github.com/my/module", "js", "js/v1.0.0"),
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/my/module/js",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "THIS IS A README",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "js",
						Path: "github.com/my/module/js/js",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package js only works with wasm.",
						GOOS:     "js",
						GOARCH:   "wasm",
					},
				},
			},
		},
	},
}

var moduleAlternative = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/my/module",
		Files:      map[string]string{"go.mod": "module canonical"},
	},
	fr: &FetchResult{
		GoModPath: "canonical",
	},
}

var moduleStd = &testModule{
	mod: &proxy.Module{
		ModulePath: stdlib.ModulePath,
		Version:    "v1.12.5",
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "std",
				Version:    "v1.12.5",
				CommitTime: stdlib.TestCommitTime,
				HasGoMod:   true,
				SourceInfo: source.NewGitHubInfo("https://github.com/golang/go", "src", "go1.12.5"),
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "std",

						IsRedistributable: true,
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "# The Go Programming Language\n",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "builtin",
						Path: "builtin",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package builtin provides documentation for Go's predeclared identifiers.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "cmd",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "main",
						Path: "cmd/pprof",
					},
					Readme: &internal.Readme{
						Filepath: "cmd/pprof/README",
						Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Pprof interprets and displays profiles of Go programs.",
					},
					Imports: []string{
						"cmd/internal/objfile",
						"crypto/tls",
						"debug/dwarf",
						"fmt",
						"github.com/google/pprof/driver",
						"github.com/google/pprof/profile",
						"golang.org/x/crypto/ssh/terminal",
						"io",
						"io/ioutil",
						"net/http",
						"net/url",
						"os",
						"regexp",
						"strconv",
						"strings",
						"sync",
						"time",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "context",
						Path: "context",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					},
					Imports: []string{"errors", "fmt", "reflect", "sync", "time"},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "encoding",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "json",
						Path: "encoding/json",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
					},
					Imports: []string{
						"bytes",
						"encoding",
						"encoding/base64",
						"errors",
						"fmt",
						"io",
						"math",
						"reflect",
						"sort",
						"strconv",
						"strings",
						"sync",
						"unicode",
						"unicode/utf16",
						"unicode/utf8",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "errors",
						Path: "errors",
					},
					Documentation: &internal.Documentation{
						Synopsis: "Package errors implements functions to manipulate errors.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "flag",
						Path: "flag",
					},
					Imports: []string{"errors", "fmt", "io", "os", "reflect", "sort", "strconv", "strings", "time"},
					Documentation: &internal.Documentation{
						Synopsis: "Package flag implements command-line flag parsing.",
					},
				},
			},
		},
	},
}

var moduleMaster = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"foo/foo.go": "// package foo exports a helpful constant.\npackage foo\nconst Bar = 1",
		},
		Version: "v0.0.0-20200706064627-355bc3f705ed",
	},
	fr: &FetchResult{
		RequestedVersion: "master",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/my/module",
				Version:    "v0.0.0-20200706064627-355bc3f705ed",
				SourceInfo: source.NewGitHubInfo("https://github.com/my/module", "", "355bc3f705ed"),
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/my/module",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "github.com/my/module/foo",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package foo exports a helpful constant.",
					},
				},
			},
		},
	},
}

var moduleLatest = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"foo/foo.go": "// package foo exports a helpful constant.\npackage foo\nconst Bar = 1",
		},
		Version: "v1.2.4",
	},
	fr: &FetchResult{
		RequestedVersion: "latest",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/my/module",
				Version:    "v1.2.4",
				SourceInfo: source.NewGitHubInfo("https://github.com/my/module", "", "v1.2.4"),
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/my/module",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "github.com/my/module/foo",
					},
					Documentation: &internal.Documentation{
						Synopsis: "package foo exports a helpful constant.",
					},
				},
			},
		},
	},
}

// moduleWithExamples returns a testModule that contains an example.
// It provides the common bits for the tests for package, function,
// type, and method examples below.
//
// The fetch result's documentation HTML is treated as a set
// of substrings that should appear in the generated documentation.
// The substrings are separated by a '~' character.
func moduleWithExamples(path, source, test string, docSubstrings ...string) *testModule {
	return &testModule{
		mod: &proxy.Module{
			ModulePath: path,
			Files: map[string]string{
				"LICENSE": testhelper.BSD0License,
				"example/example.go": `
// Package example contains examples.
package example
` + source,
				"example/example_test.go": `
package example_test
` + test,
			},
		},
		fr: &FetchResult{
			GoModPath: path,
			Module: &internal.Module{
				ModuleInfo: internal.ModuleInfo{
					ModulePath: path,
					HasGoMod:   false,
				},
				Units: []*internal.Unit{
					{
						UnitMeta: internal.UnitMeta{
							Path: path,
						},
					},
					{
						UnitMeta: internal.UnitMeta{
							Name: "example",
							Path: path + "/example",
						},
						Documentation: &internal.Documentation{
							Synopsis: "Package example contains examples.",
						},
					},
				},
			},
		},
		docStrings: map[string][]string{
			path + "/example": docSubstrings,
		},
	}
}

var modulePackageExample = moduleWithExamples("package.example",
	``,
	`import "fmt"

// Example for the package.
func Example() {
	fmt.Println("hello")
	// Output: hello
}
`, "Documentation-exampleButtonsContainer")

var moduleFuncExample = moduleWithExamples("func.example",
	`func F() {}
`, `import "func.example/example"

// Example for the function.
func ExampleF() {
	example.F()
}
`, "Documentation-exampleButtonsContainer")

var moduleTypeExample = moduleWithExamples("type.example",
	`type T struct{}
`, `import "type.example/example"

// Example for the type.
func ExampleT() {
	example.T{}
}
`, "Documentation-exampleButtonsContainer")

var moduleMethodExample = moduleWithExamples("method.example",
	`type T struct {}

func (*T) M() {}
`, `import "method.example/example"

// Example for the method.
func ExampleT_M() {
	new(example.T).M()
}
`, "Documentation-exampleButtonsContainer")
