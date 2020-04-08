// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"net/http"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/testhelper"
)

type testModule struct {
	mod *proxy.TestModule
	fr  *FetchResult
}

var moduleOnePackage = &testModule{
	mod: &proxy.TestModule{
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
				ModulePath:     "github.com/basic",
				ReadmeFilePath: "README.md",
				ReadmeContents: "THIS IS A README",
				HasGoMod:       false,
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "github.com/basic",
					V1Path: "github.com/basic",
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "THIS IS A README",
					},
				},
				{
					Path:   "github.com/basic/foo",
					V1Path: "github.com/basic/foo",
					Package: &internal.PackageNew{
						Name: "foo",
						Documentation: &internal.Documentation{
							Synopsis: "package foo exports a helpful constant.",
						},
						Imports: []string{"net/http"},
					},
				},
			},
		},
	},
}

var moduleMultiPackage = &testModule{
	mod: &proxy.TestModule{
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
				ModulePath:     "github.com/my/module",
				HasGoMod:       true,
				ReadmeFilePath: "README.md",
				ReadmeContents: "README FILE FOR TESTING.",
				SourceInfo:     source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "github.com/my/module",
					V1Path: "github.com/my/module",
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README FILE FOR TESTING.",
					},
				},
				{
					Path:   "github.com/my/module/bar",
					V1Path: "github.com/my/module/bar",
					Readme: &internal.Readme{
						Filepath: "bar/README.md",
						Contents: "Another README FILE FOR TESTING.",
					},
					Package: &internal.PackageNew{
						Name: "bar",
						Documentation: &internal.Documentation{
							Synopsis: "package bar",
							HTML:     "Bar returns the string &#34;bar&#34;.",
						},
					},
				},
				{
					Path:   "github.com/my/module/foo",
					V1Path: "github.com/my/module/foo",
					Package: &internal.PackageNew{
						Name: "foo",
						Documentation: &internal.Documentation{
							Synopsis: "package foo",
							HTML:     "FooBar returns the string &#34;foo bar&#34;.",
						},
						Imports: []string{"fmt", "github.com/my/module/bar"},
					},
				},
			},
		},
	},
}

var moduleNoGoMod = &testModule{
	mod: &proxy.TestModule{
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
			Directories: []*internal.DirectoryNew{
				{
					Path:   "no.mod/module",
					V1Path: "no.mod/module",
				},
				{
					Path:   "no.mod/module/p",
					V1Path: "no.mod/module/p",
					Package: &internal.PackageNew{
						Name: "p",
						Documentation: &internal.Documentation{
							Synopsis: "Package p is inside a module where a go.mod file hasn't been explicitly added yet.",
							HTML:     "const Year = 2009",
						},
					},
				},
			},
		},
	},
}

var moduleEmpty = &testModule{
	mod: &proxy.TestModule{
		ModulePath: "emp.ty/module",
	},
	fr: &FetchResult{Module: &internal.Module{}},
}

var moduleBadPackages = &testModule{
	mod: &proxy.TestModule{
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
		HasIncompletePackages: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "bad.mod/module",
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "bad.mod/module",
					V1Path: "bad.mod/module",
				},
				{
					Path:   "bad.mod/module/good",
					V1Path: "bad.mod/module/good",
					Package: &internal.PackageNew{
						Name: "good",
						Documentation: &internal.Documentation{
							Synopsis: "Package good is inside a module that has bad packages.",
							HTML:     `const Good = <a href="/pkg/builtin#true">true</a>`,
						},
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
}

var moduleBuildConstraints = &testModule{
	mod: &proxy.TestModule{
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
		HasIncompletePackages: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "build.constraints/module",
				HasGoMod:   false,
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "build.constraints/module",
					V1Path: "build.constraints/module",
				},
				{
					Path:   "build.constraints/module/cpu",
					V1Path: "build.constraints/module/cpu",
					Package: &internal.PackageNew{
						Name: "cpu",
						Documentation: &internal.Documentation{
							Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
							HTML:     "const CacheLinePadSize = 3",
						},
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
				Status:      derrors.ToHTTPStatus(derrors.BuildContextNotSupported),
			},
		},
	},
}

var moduleNonRedist = &testModule{
	mod: &proxy.TestModule{
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
		HasIncompletePackages: false,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:     "nonredistributable.mod/module",
				ReadmeFilePath: "README.md",
				ReadmeContents: "README FILE FOR TESTING.",
				HasGoMod:       true,
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "nonredistributable.mod/module",
					V1Path: "nonredistributable.mod/module",
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README FILE FOR TESTING.",
					},
				},
				{
					Path:   "nonredistributable.mod/module/bar",
					V1Path: "nonredistributable.mod/module/bar",
					Package: &internal.PackageNew{
						Name: "bar",
						Documentation: &internal.Documentation{
							Synopsis: "package bar",
							HTML:     "Bar returns the string",
						},
					},
				},
				{
					Path:   "nonredistributable.mod/module/bar/baz",
					V1Path: "nonredistributable.mod/module/bar/baz",
					Package: &internal.PackageNew{
						Name: "baz",
						Documentation: &internal.Documentation{
							Synopsis: "package baz",
							HTML:     "Baz returns the string",
						},
					},
				},
				{
					Path:   "nonredistributable.mod/module/foo",
					V1Path: "nonredistributable.mod/module/foo",
					Readme: &internal.Readme{
						Filepath: "foo/README.md",
						Contents: "README FILE SHOW UP HERE BUT WILL BE REMOVED BEFORE DB INSERT",
					},
					Package: &internal.PackageNew{
						Name: "foo",
						Documentation: &internal.Documentation{
							Synopsis: "package foo",
							HTML:     "FooBar returns the string",
						},
						Imports: []string{"fmt", "github.com/my/module/bar"},
					},
				},
			},
		},
	},
}

var moduleBadImportPath = &testModule{
	mod: &proxy.TestModule{
		ModulePath: "bad.import.path.com",
		Files: map[string]string{
			"good/import/path/foo.go": "package foo",
			"bad/import path/foo.go":  "package foo",
		},
	},
	fr: &FetchResult{
		HasIncompletePackages: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "bad.import.path.com",
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "bad.import.path.com",
					V1Path: "bad.import.path.com",
				},
				{
					Path:   "bad.import.path.com/good",
					V1Path: "bad.import.path.com/good",
				},
				{
					Path:   "bad.import.path.com/good/import",
					V1Path: "bad.import.path.com/good/import",
				},
				{
					Path:   "bad.import.path.com/good/import/path",
					V1Path: "bad.import.path.com/good/import/path",
					Package: &internal.PackageNew{
						Name:          "foo",
						Documentation: &internal.Documentation{},
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				ModulePath:  "bad.import.path.com",
				PackagePath: "bad.import.path.com/bad/import path",
				Version:     "v1.0.0",
				Status:      derrors.ToHTTPStatus(derrors.BadImportPath),
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
	mod: &proxy.TestModule{
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
			Directories: []*internal.DirectoryNew{
				{
					Path:   "doc.test",
					V1Path: "doc.test",
				},
				{
					Path:   "doc.test/permalink",
					V1Path: "doc.test/permalink",
					Package: &internal.PackageNew{
						Name: "permalink",
						Documentation: &internal.Documentation{
							Synopsis: "Package permalink is for testing the heading permalink documentation rendering feature.",
							HTML:     "<h3 id=\"hdr-This_is_a_heading\">This is a heading <a href=\"#hdr-This_is_a_heading\">¶</a></h3>",
						},
					},
				},
			},
		},
	},
}

var moduleWasm = &testModule{
	mod: &proxy.TestModule{
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
				ModulePath:     "github.com/my/module/js",
				ReadmeFilePath: "README.md",
				ReadmeContents: "THIS IS A README",
				SourceInfo:     source.NewGitHubInfo("https://github.com/my/module", "js", "js/v1.0.0"),
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:   "github.com/my/module/js",
					V1Path: "github.com/my/module/js",
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "THIS IS A README",
					},
				},
				{

					Path:   "github.com/my/module/js/js",
					V1Path: "github.com/my/module/js/js",
					Package: &internal.PackageNew{
						Name: "js",
						Documentation: &internal.Documentation{
							Synopsis: "Package js only works with wasm.",
							GOOS:     "js",
							GOARCH:   "wasm",
						},
					},
				},
			},
		},
	},
}

var moduleAlternative = &testModule{
	mod: &proxy.TestModule{
		ModulePath: "github.com/my/module",
		Files:      map[string]string{"go.mod": "module canonical"},
	},
	fr: &FetchResult{
		GoModPath: "canonical",
	},
}

var moduleStd = &testModule{
	mod: &proxy.TestModule{
		ModulePath: stdlib.ModulePath,
		Version:    "v1.12.5",
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:     "std",
				Version:        "v1.12.5",
				CommitTime:     stdlib.TestCommitTime,
				ReadmeFilePath: "README.md",
				ReadmeContents: "# The Go Programming Language\n",
				HasGoMod:       true,
				SourceInfo:     source.NewGitHubInfo("https://github.com/golang/go", "src", "go1.12.5"),
			},
			Directories: []*internal.DirectoryNew{
				{
					Path:              "std",
					V1Path:            "std",
					IsRedistributable: true,
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "# The Go Programming Language\n",
					},
				},
				{
					Path:   "builtin",
					V1Path: "builtin",
					Package: &internal.PackageNew{
						Name: "builtin",
						Documentation: &internal.Documentation{
							Synopsis: "Package builtin provides documentation for Go's predeclared identifiers.",
						},
					},
				},
				{
					Path:   "cmd",
					V1Path: "cmd",
				},
				{
					Path:   "cmd/pprof",
					V1Path: "cmd/pprof",
					Readme: &internal.Readme{
						Filepath: "cmd/pprof/README",
						Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
					},
					Package: &internal.PackageNew{
						Name: "main",
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
				},
				{
					Path:   "context",
					V1Path: "context",
					Package: &internal.PackageNew{
						Name: "context",
						Documentation: &internal.Documentation{
							Synopsis: "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
						},
						Imports: []string{"errors", "fmt", "reflect", "sync", "time"},
					},
				},
				{
					Path:   "encoding",
					V1Path: "encoding",
				},
				{
					Path:   "encoding/json",
					V1Path: "encoding/json",
					Package: &internal.PackageNew{
						Name: "json",
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
				},
				{
					Path:   "errors",
					V1Path: "errors",
					Package: &internal.PackageNew{
						Name: "errors",
						Documentation: &internal.Documentation{
							Synopsis: "Package errors implements functions to manipulate errors.",
						},
					},
				},
				{
					Path:   "flag",
					V1Path: "flag",
					Package: &internal.PackageNew{
						Name: "flag",
						Documentation: &internal.Documentation{
							Synopsis: "Package flag implements command-line flag parsing.",
						},
						Imports: []string{"errors", "fmt", "io", "os", "reflect", "sort", "strconv", "strings", "time"},
					},
				},
			},
		},
	},
}

// moduleWithExamples returns a testModule that contains an example.
// It provides the common bits for the tests for package, function,
// type, and method examples below.
func moduleWithExamples(path, source, test string) *testModule {
	return &testModule{
		mod: &proxy.TestModule{
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
				Directories: []*internal.DirectoryNew{
					{
						Path:   path,
						V1Path: path,
					},
					{
						Path:   path + "/example",
						V1Path: path + "/example",
						Package: &internal.PackageNew{
							Name: "example",
							Documentation: &internal.Documentation{
								Synopsis: "Package example contains examples.",
								HTML:     testPlaygroundID,
							},
						},
					},
				},
			},
		},
	}
}

const testPlaygroundID = "playground-id"

var modulePackageExample = moduleWithExamples("package.example",
	``,
	`import "fmt"

// Example for the package.
func Example() {
	fmt.Println("hello")
	// Output: hello
}
`)

var moduleFuncExample = moduleWithExamples("func.example",
	`func F() {}
`, `import "func.example/example"

// Example for the function.
func ExampleF() {
	example.F()
}
`)

var moduleTypeExample = moduleWithExamples("type.example",
	`type T struct{}
`, `import "type.example/example"

// Example for the type.
func ExampleT() {
	example.T{}
}
`)

var moduleMethodExample = moduleWithExamples("method.example",
	`type T struct {}

func (*T) M() {}
`, `import "method.example/example"

// Example for the method.
func ExampleT_M() {
	new(example.T).M()
}
`)
