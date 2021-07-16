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
	modfunc    func() *proxy.Module // call to get module if mod field is nil
	fr         *FetchResult
	docStrings map[string][]string
}

var singleUnits = []*internal.Unit{
	{
		UnitMeta: internal.UnitMeta{
			Path: "example.com/single",
		},
		Readme: &internal.Readme{
			Filepath: "README.md",
			Contents: "This is the README for a test module.",
		},
	},
	{
		UnitMeta: internal.UnitMeta{
			Name: "pkg",
			Path: "example.com/single/pkg",
		},
		Documentation: []*internal.Documentation{{
			GOOS:     internal.All,
			GOARCH:   internal.All,
			Synopsis: "Package pkg is a sample package.",
			API: []*internal.Symbol{
				{
					SymbolMeta: internal.SymbolMeta{
						Name:     "Version",
						Synopsis: "const Version",
						Section:  "Constants",
						Kind:     "Constant",
					},
				},
				{
					SymbolMeta: internal.SymbolMeta{
						Name:     "V",
						Synopsis: "var V = Version",
						Section:  "Variables",
						Kind:     "Variable",
					},
				},
				{
					SymbolMeta: internal.SymbolMeta{
						Name:     "G",
						Synopsis: "func G() int",
						Section:  "Functions",
						Kind:     "Function",
					},
				},
				{
					SymbolMeta: internal.SymbolMeta{
						Name:     "T",
						Synopsis: "type T int",
						Section:  "Types",
						Kind:     "Type",
					},
					Children: []*internal.SymbolMeta{
						{
							Name:       "F",
							Synopsis:   "func F(t time.Time, s string) (T, u)",
							Section:    "Types",
							Kind:       "Function",
							ParentName: "T",
						},
					},
				},
			},
		}},
		Imports: []string{"time"},
	},
}

var moduleOnePackage = &testModule{
	modfunc: func() *proxy.Module {
		return proxy.FindModule(testModules, "example.com/single", "v1.0.0")
	},
	fr: &FetchResult{
		HasGoMod: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/single",
				HasGoMod:          true,
				SourceInfo:        source.NewGitHubInfo("https://example.com/single", "", "v1.0.0"),
				IsRedistributable: true,
			},
			Units: singleUnits,
		},
	},
}

var moduleNoGoMod = &testModule{
	modfunc: func() *proxy.Module {
		return proxy.FindModule(testModules, "example.com/basic", "v1.0.0").
			ChangePath("example.com/nogo").
			DeleteFile("go.mod")
	},
	fr: &FetchResult{
		HasGoMod: false,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/nogo",
				HasGoMod:          false,
				SourceInfo:        source.NewGitHubInfo("https://example.com/nogo", "", "v1.0.0"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Name: "basic",
						Path: "example.com/nogo",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "This is the README for a test module.",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package basic is a sample package.",
							API:      singleUnits[1].Documentation[0].API,
						},
					},
					Imports: []string{"time"},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"no.mod/module/p": {"const Year = 2009"},
	},
}

var moduleMultiPackage = &testModule{
	modfunc: func() *proxy.Module { return proxy.FindModule(testModules, "example.com/multi", "v1.0.0") },
	fr: &FetchResult{
		HasGoMod: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/multi",
				HasGoMod:          true,
				SourceInfo:        source.NewGitHubInfo("https://example.com/multi", "", "v1.0.0"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com/multi",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README file for testing.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "bar",
						Path: "example.com/multi/bar",
					},
					Readme: &internal.Readme{
						Filepath: "bar/README",
						Contents: "Another README file for testing.",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "package bar",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Bar",
										Synopsis: "func Bar() string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
							},
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "foo",
						Path: "example.com/multi/foo",
					},
					Documentation: []*internal.Documentation{{
						GOOS:     internal.All,
						GOARCH:   internal.All,
						Synopsis: "package foo",
						API: []*internal.Symbol{
							{
								SymbolMeta: internal.SymbolMeta{
									Name:     "FooBar",
									Synopsis: "func FooBar() string",
									Section:  "Functions",
									Kind:     "Function",
								},
							},
						},
					}},
					Imports: []string{"example.com/multi/bar", "fmt"},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"github.com/my/module/bar": {"Bar returns the string &#34;bar&#34;."},
		"github.com/my/module/foo": {"FooBar returns the string &#34;foo bar&#34;."},
	},
}

var moduleEmpty = &testModule{
	mod: &proxy.Module{
		ModulePath: "emp.ty/module",
	},
	fr: &FetchResult{Module: &internal.Module{}},
}

var moduleNoGo = &testModule{
	mod: &proxy.Module{
		ModulePath: "no.go/files",
		Files:      map[string]string{"go.mod": "module no.go/files"},
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
				ModulePath:        "bad.mod/module",
				IsRedistributable: true,
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
					Documentation: []*internal.Documentation{{
						GOOS:     internal.All,
						GOARCH:   internal.All,
						Synopsis: "Package good is inside a module that has bad packages.",
						API: []*internal.Symbol{{
							SymbolMeta: internal.SymbolMeta{
								Name:     "Good",
								Synopsis: "const Good",
								Section:  "Constants",
								Kind:     "Constant",
							},
						}},
					}},
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
	modfunc: func() *proxy.Module { return proxy.FindModule(testModules, "example.com/build-constraints", "") },
	fr: &FetchResult{
		Status:   derrors.ToStatus(derrors.HasIncompletePackages),
		HasGoMod: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/build-constraints",
				HasGoMod:          true,
				SourceInfo:        source.NewGitHubInfo("https://example.com/build-constraints", "", "v1.0.0"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com/build-constraints",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "cpu",
						Path: "example.com/build-constraints/cpu",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     "linux",
							GOARCH:   "amd64",
							Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "CacheLinePadSize",
										Synopsis: "const CacheLinePadSize",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
							},
						},
						{
							GOOS:     "windows",
							GOARCH:   "amd64",
							Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "CacheLinePadSize",
										Synopsis: "const CacheLinePadSize",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
							},
						},
						{
							GOOS:     "darwin",
							GOARCH:   "amd64",
							Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "CacheLinePadSize",
										Synopsis: "const CacheLinePadSize",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
							},
						},
						{
							GOOS:     "js",
							GOARCH:   "wasm",
							Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
						},
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				ModulePath:  "example.com/build-constraints",
				Version:     "v1.0.0",
				PackagePath: "example.com/build-constraints/cpu",
				Status:      http.StatusOK,
			},
			{
				ModulePath:  "example.com/build-constraints",
				Version:     "v1.0.0",
				PackagePath: "example.com/build-constraints/ignore",
				Status:      derrors.ToStatus(derrors.PackageBuildContextNotSupported),
			},
		},
	},
	docStrings: map[string][]string{
		"build.constraints/module/cpu": {"const CacheLinePadSize = 3"},
	},
}

// The package in this module is broken for one build context, but not all.
var moduleBadBuildContext = &testModule{
	mod: &proxy.Module{
		ModulePath: "github.com/bad-context",
		Files: map[string]string{
			"pkg/linux.go": `
					// +build linux js

					package pkg`,
			"pkg/js.go": `
					// +build js

					package js`,
		},
	},
	fr: &FetchResult{
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "github.com/bad-context",
				HasGoMod:          false,
				IsRedistributable: false,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "github.com/bad-context",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "pkg",
						Path: "github.com/bad-context/pkg",
					},
					Documentation: []*internal.Documentation{{
						GOOS:   "linux",
						GOARCH: "amd64",
					}},
				},
			},
		},
	},
}

var moduleNonRedist = &testModule{
	modfunc: func() *proxy.Module { return proxy.FindModule(testModules, "example.com/nonredist", "") },
	fr: &FetchResult{
		HasGoMod: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/nonredist",
				HasGoMod:          true,
				SourceInfo:        source.NewGitHubInfo("https://example.com/nonredist", "", "v1.0.0"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com/nonredist",
					},
					Readme: &internal.Readme{
						Filepath: "README.md",
						Contents: "README file for testing.",
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "bar",
						Path: "example.com/nonredist/bar",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "package bar",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Bar",
										Synopsis: "func Bar() string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
							},
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "baz",
						Path: "example.com/nonredist/bar/baz",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "package baz",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Baz",
										Synopsis: "func Baz() string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
							},
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "unk",
						Path: "example.com/nonredist/unk",
					},
					Readme: &internal.Readme{
						Filepath: "unk/README.md",
						Contents: "README file will be removed before DB insert.",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "package unk",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "FooBar",
										Synopsis: "func FooBar() string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
							},
						},
					},
					Imports: []string{"example.com/nonredist/bar", "fmt"},
				},
			},
		},
	},
	docStrings: map[string][]string{
		"example.com/nonredist/bar":     {"Bar returns the string"},
		"example.com/nonredist/bar/baz": {"Baz returns the string"},
		"example.com/nonredist/foo":     {"FooBar returns the string"},
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
					Documentation: []*internal.Documentation{{GOOS: internal.All, GOARCH: internal.All}},
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
		HasGoMod:  false,
		GoModPath: "doc.test",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "doc.test",
				HasGoMod:          false,
				IsRedistributable: true,
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
					Documentation: []*internal.Documentation{{
						GOOS:     internal.All,
						GOARCH:   internal.All,
						Synopsis: "Package permalink is for testing the heading permalink documentation rendering feature.",
					}},
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
				strings.Repeat("// Too big.\n", 100_000) +
				"package bigdoc",
		},
	},
	fr: &FetchResult{
		HasGoMod:  false,
		GoModPath: "bigdoc.test",
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "bigdoc.test",
				HasGoMod:          false,
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Name: "bigdoc",
						Path: "bigdoc.test",
					},
					Documentation: []*internal.Documentation{{
						GOOS:     internal.All,
						GOARCH:   internal.All,
						Synopsis: "This documentation is big.",
					}},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				PackagePath: "bigdoc.test",
				ModulePath:  "bigdoc.test",
				Version:     "v1.0.0",
				Status:      200,
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
				ModulePath:        "github.com/my/module/js",
				SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "js", "js/v1.0.0"),
				IsRedistributable: true,
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
					Documentation: []*internal.Documentation{
						{
							Synopsis: "Package js only works with wasm.",
							GOOS:     "js",
							GOARCH:   "wasm",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{Name: "Value",
										Synopsis: "type Value int",
										Section:  "Types",
										Kind:     "Type",
									},
								},
							},
						},
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

var moduleStdMaster = &testModule{
	mod: &proxy.Module{
		ModulePath: stdlib.ModulePath,
		Version:    "master",
		// No files necessary because the internal/stdlib package will read from
		// internal/stdlib/testdata.
	},
	fr: &FetchResult{
		RequestedVersion: "master",
		ResolvedVersion:  stdlib.TestMasterVersion,
		HasGoMod:         true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        stdlib.ModulePath,
				Version:           stdlib.TestMasterVersion,
				CommitTime:        stdlib.TestCommitTime,
				HasGoMod:          true,
				SourceInfo:        source.NewStdlibInfo("master"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path:              "errors",
						Name:              "errors",
						IsRedistributable: true,
						ModuleInfo: internal.ModuleInfo{
							Version:           stdlib.TestMasterVersion,
							ModulePath:        stdlib.ModulePath,
							IsRedistributable: true,
						},
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package errors implements functions to manipulate errors.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "New",
										Synopsis: "func New(text string) error",
										Section:  "Functions",
										Kind:     "Function",
									},
									GOOS:   internal.All,
									GOARCH: internal.All,
								},
							},
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path:              "std",
						IsRedistributable: true,
						ModuleInfo: internal.ModuleInfo{
							Version:           stdlib.TestMasterVersion,
							ModulePath:        "std",
							IsRedistributable: true,
						},
					},
				},
			},
		},
		PackageVersionStates: []*internal.PackageVersionState{
			{
				PackagePath: "errors",
				ModulePath:  "std",
				Version:     stdlib.TestMasterVersion,
				Status:      200,
			},
		},
	},
}

var moduleStd = &testModule{
	mod: &proxy.Module{
		ModulePath: stdlib.ModulePath,
		Version:    "v1.12.5",
		// No files necessary because the internal/stdlib package will read from
		// internal/stdlib/testdata.
	},
	fr: &FetchResult{
		HasGoMod: true,
		Module: &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "std",
				Version:           "v1.12.5",
				CommitTime:        stdlib.TestCommitTime,
				HasGoMod:          true,
				SourceInfo:        source.NewStdlibInfo("v1.12.5"),
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path:              "std",
						IsRedistributable: true,
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "builtin",
						Path: "builtin",
					},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package builtin provides documentation for Go's predeclared identifiers.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "true",
										Synopsis: "const true",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "false",
										Synopsis: "const false",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "iota",
										Synopsis: "const iota",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "nil",
										Synopsis: "var nil Type",
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "append",
										Synopsis: "func append(slice []Type, elems ...Type) []Type",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "cap",
										Synopsis: "func cap(v Type) int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "close",
										Synopsis: "func close(c chan<- Type)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "complex",
										Synopsis: "func complex(r, i FloatType) ComplexType",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "copy",
										Synopsis: "func copy(dst, src []Type) int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "delete",
										Synopsis: "func delete(m map[Type]Type1, key Type)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "imag",
										Synopsis: "func imag(c ComplexType) FloatType",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "len",
										Synopsis: "func len(v Type) int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "make",
										Synopsis: "func make(t Type, size ...IntegerType) Type",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "new",
										Synopsis: "func new(Type) *Type",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "panic",
										Synopsis: "func panic(v interface{})",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "print",
										Synopsis: "func print(args ...Type)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "println",
										Synopsis: "func println(args ...Type)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "real",
										Synopsis: "func real(c ComplexType) FloatType",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "recover",
										Synopsis: "func recover() interface{}",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "ComplexType",
										Synopsis: "type ComplexType complex64",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "FloatType",
										Synopsis: "type FloatType float32",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "IntegerType",
										Synopsis: "type IntegerType int",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Type",
										Synopsis: "type Type int",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Type1",
										Synopsis: "type Type1 int",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "bool",
										Synopsis: "type bool bool",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "byte",
										Synopsis: "type byte = uint8",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "complex128",
										Synopsis: "type complex128 complex128",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "complex64",
										Synopsis: "type complex64 complex64",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "error",
										Synopsis: "type error interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "error.Error",
											Synopsis:   "Error func() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "error",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "float32",
										Synopsis: "type float32 float32",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "float64",
										Synopsis: "type float64 float64",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "int",
										Synopsis: "type int int",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "int16",
										Synopsis: "type int16 int16",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "int32",
										Synopsis: "type int32 int32",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "int64",
										Synopsis: "type int64 int64",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "int8",
										Synopsis: "type int8 int8",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "rune",
										Synopsis: "type rune = int32",
										Section:  "Types",
										Kind:     "Type"},
								},

								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "string",
										Synopsis: "type string string",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uint",
										Synopsis: "type uint uint",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uint16",
										Synopsis: "type uint16 uint16",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uint32",
										Synopsis: "type uint32 uint32",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uint64",
										Synopsis: "type uint64 uint64",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uint8",
										Synopsis: "type uint8 uint8",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "uintptr",
										Synopsis: "type uintptr uintptr",
										Section:  "Types",
										Kind:     "Type",
									},
								},
							},
						},
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
					// cmd/pprof has a file with a build constraint that does not include js/wasm.
					// Since the set files isn't the same across all build contexts, we represent
					// every build context.
					Documentation: []*internal.Documentation{
						{
							GOOS:     "linux",
							GOARCH:   "amd64",
							Synopsis: "Pprof interprets and displays profiles of Go programs.",
						},
						{
							GOOS:     "windows",
							GOARCH:   "amd64",
							Synopsis: "Pprof interprets and displays profiles of Go programs.",
						},
						{
							GOOS:     "darwin",
							GOARCH:   "amd64",
							Synopsis: "Pprof interprets and displays profiles of Go programs.",
						},
						{
							GOOS:     "js",
							GOARCH:   "wasm",
							Synopsis: "Pprof interprets and displays profiles of Go programs.",
						},
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
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Canceled",
										Synopsis: `var Canceled = errors.New("context canceled")`,
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "DeadlineExceeded",
										Synopsis: "var DeadlineExceeded error = deadlineExceededError{}",
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "WithCancel",
										Synopsis: "func WithCancel(parent Context) (ctx Context, cancel CancelFunc)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "WithDeadline",
										Synopsis: "func WithDeadline(parent Context, d time.Time) (Context, CancelFunc)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "WithTimeout",
										Synopsis: "func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "CancelFunc",
										Synopsis: "type CancelFunc func()",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Context",
										Synopsis: "type Context interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Background",
											Synopsis:   "func Background() Context",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Context",
										},
										{
											Name:       "TODO",
											Synopsis:   "func TODO() Context",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Context",
										},
										{
											Name:       "WithValue",
											Synopsis:   "func WithValue(parent Context, key, val interface{}) Context",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Context",
										},
										{
											Name:       "Context.Deadline",
											Synopsis:   "Deadline func() (deadline time.Time, ok bool)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Context",
										},
										{
											Name:       "Context.Done",
											Synopsis:   "Done func() <-chan struct{}",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Context",
										},
										{
											Name:       "Context.Err",
											Synopsis:   "Err func() error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Context",
										},
										{
											Name:       "Context.Value",
											Synopsis:   "Value func(key interface{}) interface{}",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Context",
										},
									},
								},
							},
						},
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
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Compact",
										Synopsis: "func Compact(dst *bytes.Buffer, src []byte) error",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "HTMLEscape",
										Synopsis: "func HTMLEscape(dst *bytes.Buffer, src []byte)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Indent",
										Synopsis: "func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Marshal",
										Synopsis: "func Marshal(v interface{}) ([]byte, error)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "MarshalIndent",
										Synopsis: "func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Unmarshal",
										Synopsis: "func Unmarshal(data []byte, v interface{}) error",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Valid",
										Synopsis: "func Valid(data []byte) bool",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Decoder",
										Synopsis: "type Decoder struct{}",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "NewDecoder",
											Synopsis:   "func NewDecoder(r io.Reader) *Decoder",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.Buffered",
											Synopsis:   "func (dec *Decoder) Buffered() io.Reader",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.Decode",
											Synopsis:   "func (dec *Decoder) Decode(v interface{}) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.DisallowUnknownFields",
											Synopsis:   "func (dec *Decoder) DisallowUnknownFields()",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.More",
											Synopsis:   "func (dec *Decoder) More() bool",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.Token",
											Synopsis:   "func (dec *Decoder) Token() (Token, error)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
										{
											Name:       "Decoder.UseNumber",
											Synopsis:   "func (dec *Decoder) UseNumber()",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Decoder",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Delim",
										Synopsis: "type Delim rune",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Delim.String",
											Synopsis:   "func (d Delim) String() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Delim",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Encoder",
										Synopsis: "type Encoder struct{}",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "NewEncoder",
											Synopsis:   "func NewEncoder(w io.Writer) *Encoder",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Encoder",
										},
										{
											Name:       "Encoder.Encode",
											Synopsis:   "func (enc *Encoder) Encode(v interface{}) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Encoder",
										},
										{
											Name:       "Encoder.SetEscapeHTML",
											Synopsis:   "func (enc *Encoder) SetEscapeHTML(on bool)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Encoder",
										},
										{
											Name:       "Encoder.SetIndent",
											Synopsis:   "func (enc *Encoder) SetIndent(prefix, indent string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Encoder",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "InvalidUTF8Error",
										Synopsis: "type InvalidUTF8Error struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "InvalidUTF8Error.S",
											Synopsis:   "S string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "InvalidUTF8Error",
										},
										{
											Name:       "InvalidUTF8Error.Error",
											Synopsis:   "func (e *InvalidUTF8Error) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "InvalidUTF8Error",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "InvalidUnmarshalError",
										Synopsis: "type InvalidUnmarshalError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "InvalidUnmarshalError.Type",
											Synopsis:   "Type reflect.Type",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "InvalidUnmarshalError",
										},
										{
											Name:       "InvalidUnmarshalError.Error",
											Synopsis:   "func (e *InvalidUnmarshalError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "InvalidUnmarshalError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Marshaler",
										Synopsis: "type Marshaler interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Marshaler.MarshalJSON",
											Synopsis:   "MarshalJSON func() ([]byte, error)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Marshaler",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "MarshalerError",
										Synopsis: "type MarshalerError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "MarshalerError.Type",
											Synopsis:   "Type reflect.Type",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "MarshalerError",
										},
										{
											Name:       "MarshalerError.Err",
											Synopsis:   "Err error",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "MarshalerError",
										},
										{
											Name:       "MarshalerError.Error",
											Synopsis:   "func (e *MarshalerError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "MarshalerError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Number",
										Synopsis: "type Number string",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Number.Float64",
											Synopsis:   "func (n Number) Float64() (float64, error)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Number",
										},
										{
											Name:       "Number.Int64",
											Synopsis:   "func (n Number) Int64() (int64, error)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Number",
										},
										{
											Name:       "Number.String",
											Synopsis:   "func (n Number) String() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Number",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "RawMessage",
										Synopsis: "type RawMessage []byte",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "RawMessage.MarshalJSON",
											Synopsis:   "func (m RawMessage) MarshalJSON() ([]byte, error)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "RawMessage",
										},
										{
											Name:       "RawMessage.UnmarshalJSON",
											Synopsis:   "func (m *RawMessage) UnmarshalJSON(data []byte) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "RawMessage",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "SyntaxError",
										Synopsis: "type SyntaxError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "SyntaxError.Offset",
											Synopsis:   "Offset int64",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "SyntaxError",
										},
										{
											Name:       "SyntaxError.Error",
											Synopsis:   "func (e *SyntaxError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "SyntaxError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Token",
										Synopsis: "type Token interface{}",
										Section:  "Types",
										Kind:     "Type",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UnmarshalFieldError",
										Synopsis: "type UnmarshalFieldError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "UnmarshalFieldError.Key",
											Synopsis:   "Key string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalFieldError",
										},
										{
											Name:       "UnmarshalFieldError.Type",
											Synopsis:   "Type reflect.Type",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalFieldError",
										},
										{
											Name:       "UnmarshalFieldError.Field",
											Synopsis:   "Field reflect.StructField",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalFieldError",
										},
										{
											Name:       "UnmarshalFieldError.Error",
											Synopsis:   "func (e *UnmarshalFieldError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "UnmarshalFieldError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UnmarshalTypeError",
										Synopsis: "type UnmarshalTypeError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "UnmarshalTypeError.Value",
											Synopsis:   "Value string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalTypeError",
										},
										{
											Name:       "UnmarshalTypeError.Type",
											Synopsis:   "Type reflect.Type",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalTypeError",
										},
										{
											Name:       "UnmarshalTypeError.Offset",
											Synopsis:   "Offset int64",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalTypeError",
										},
										{
											Name:       "UnmarshalTypeError.Struct",
											Synopsis:   "Struct string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalTypeError",
										},
										{
											Name:       "UnmarshalTypeError.Field",
											Synopsis:   "Field string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnmarshalTypeError",
										},
										{
											Name:       "UnmarshalTypeError.Error",
											Synopsis:   "func (e *UnmarshalTypeError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "UnmarshalTypeError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Unmarshaler",
										Synopsis: "type Unmarshaler interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Unmarshaler.UnmarshalJSON",
											Synopsis:   "UnmarshalJSON func([]byte) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Unmarshaler",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UnsupportedTypeError",
										Synopsis: "type UnsupportedTypeError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "UnsupportedTypeError.Type",
											Synopsis:   "Type reflect.Type",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnsupportedTypeError",
										},
										{
											Name:       "UnsupportedTypeError.Error",
											Synopsis:   "func (e *UnsupportedTypeError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "UnsupportedTypeError",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UnsupportedValueError",
										Synopsis: "type UnsupportedValueError struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "UnsupportedValueError.Value",
											Synopsis:   "Value reflect.Value",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnsupportedValueError",
										},
										{
											Name:       "UnsupportedValueError.Str",
											Synopsis:   "Str string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "UnsupportedValueError",
										},
										{
											Name:       "UnsupportedValueError.Error",
											Synopsis:   "func (e *UnsupportedValueError) Error() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "UnsupportedValueError",
										},
									},
								},
							},
						},
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
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package errors implements functions to manipulate errors.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "New",
										Synopsis: "func New(text string) error",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
							},
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Name: "flag",
						Path: "flag",
					},
					Imports: []string{"errors", "fmt", "io", "os", "reflect", "sort", "strconv", "strings", "time"},
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package flag implements command-line flag parsing.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "CommandLine",
										Synopsis: "var CommandLine = NewFlagSet(os.Args[0], ExitOnError)",
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "ErrHelp",
										Synopsis: `var ErrHelp = errors.New("flag: help requested")`,
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Usage",
										Synopsis: "var Usage = func() { ... }",
										Section:  "Variables",
										Kind:     "Variable",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Arg",
										Synopsis: "func Arg(i int) string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Args",
										Synopsis: "func Args() []string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Bool",
										Synopsis: "func Bool(name string, value bool, usage string) *bool",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "BoolVar",
										Synopsis: "func BoolVar(p *bool, name string, value bool, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Duration",
										Synopsis: "func Duration(name string, value time.Duration, usage string) *time.Duration",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "DurationVar",
										Synopsis: "func DurationVar(p *time.Duration, name string, value time.Duration, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Float64",
										Synopsis: "func Float64(name string, value float64, usage string) *float64",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Float64Var",
										Synopsis: "func Float64Var(p *float64, name string, value float64, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Int",
										Synopsis: "func Int(name string, value int, usage string) *int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Int64",
										Synopsis: "func Int64(name string, value int64, usage string) *int64",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Int64Var",
										Synopsis: "func Int64Var(p *int64, name string, value int64, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "IntVar",
										Synopsis: "func IntVar(p *int, name string, value int, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "NArg",
										Synopsis: "func NArg() int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "NFlag",
										Synopsis: "func NFlag() int",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Parse",
										Synopsis: "func Parse()",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Parsed",
										Synopsis: "func Parsed() bool",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "PrintDefaults",
										Synopsis: "func PrintDefaults()",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Set",
										Synopsis: "func Set(name, value string) error",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "String",
										Synopsis: "func String(name string, value string, usage string) *string",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "StringVar",
										Synopsis: "func StringVar(p *string, name string, value string, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Uint",
										Synopsis: "func Uint(name string, value uint, usage string) *uint",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Uint64",
										Synopsis: "func Uint64(name string, value uint64, usage string) *uint64",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Uint64Var",
										Synopsis: "func Uint64Var(p *uint64, name string, value uint64, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UintVar",
										Synopsis: "func UintVar(p *uint, name string, value uint, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "UnquoteUsage",
										Synopsis: "func UnquoteUsage(flag *Flag) (name string, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Var",
										Synopsis: "func Var(value Value, name string, usage string)",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Visit",
										Synopsis: "func Visit(fn func(*Flag))",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "VisitAll",
										Synopsis: "func VisitAll(fn func(*Flag))",
										Section:  "Functions",
										Kind:     "Function",
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "ErrorHandling",
										Synopsis: "type ErrorHandling int",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "ContinueOnError",
											Synopsis:   "const ContinueOnError",
											Section:    "Types",
											Kind:       "Constant",
											ParentName: "ErrorHandling",
										},
										{
											Name:       "ExitOnError",
											Synopsis:   "const ExitOnError",
											Section:    "Types",
											Kind:       "Constant",
											ParentName: "ErrorHandling",
										},
										{
											Name:       "PanicOnError",
											Synopsis:   "const PanicOnError",
											Section:    "Types",
											Kind:       "Constant",
											ParentName: "ErrorHandling",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Flag",
										Synopsis: "type Flag struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Lookup",
											Synopsis:   "func Lookup(name string) *Flag",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "Flag",
										},
										{
											Name:       "Flag.Name",
											Synopsis:   "Name string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "Flag",
										},
										{
											Name:       "Flag.Usage",
											Synopsis:   "Usage string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "Flag",
										},
										{
											Name:       "Flag.Value",
											Synopsis:   "Value Value",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "Flag",
										},
										{
											Name:       "Flag.DefValue",
											Synopsis:   "DefValue string",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "Flag",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "FlagSet",
										Synopsis: "type FlagSet struct{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "NewFlagSet",
											Synopsis:   "func NewFlagSet(name string, errorHandling ErrorHandling) *FlagSet",
											Section:    "Types",
											Kind:       "Function",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Usage",
											Synopsis:   "Usage func()",
											Section:    "Types",
											Kind:       "Field",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Arg",
											Synopsis:   "func (f *FlagSet) Arg(i int) string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Args",
											Synopsis:   "func (f *FlagSet) Args() []string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Bool",
											Synopsis:   "func (f *FlagSet) Bool(name string, value bool, usage string) *bool",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.BoolVar",
											Synopsis:   "func (f *FlagSet) BoolVar(p *bool, name string, value bool, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Duration",
											Synopsis:   "func (f *FlagSet) Duration(name string, value time.Duration, usage string) *time.Duration",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.DurationVar",
											Synopsis:   "func (f *FlagSet) DurationVar(p *time.Duration, name string, value time.Duration, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.ErrorHandling",
											Synopsis:   "func (f *FlagSet) ErrorHandling() ErrorHandling",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Float64",
											Synopsis:   "func (f *FlagSet) Float64(name string, value float64, usage string) *float64",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Float64Var",
											Synopsis:   "func (f *FlagSet) Float64Var(p *float64, name string, value float64, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Init",
											Synopsis:   "func (f *FlagSet) Init(name string, errorHandling ErrorHandling)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Int",
											Synopsis:   "func (f *FlagSet) Int(name string, value int, usage string) *int",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Int64",
											Synopsis:   "func (f *FlagSet) Int64(name string, value int64, usage string) *int64",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Int64Var",
											Synopsis:   "func (f *FlagSet) Int64Var(p *int64, name string, value int64, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.IntVar",
											Synopsis:   "func (f *FlagSet) IntVar(p *int, name string, value int, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Lookup",
											Synopsis:   "func (f *FlagSet) Lookup(name string) *Flag",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.NArg",
											Synopsis:   "func (f *FlagSet) NArg() int",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.NFlag",
											Synopsis:   "func (f *FlagSet) NFlag() int",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Name",
											Synopsis:   "func (f *FlagSet) Name() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Output",
											Synopsis:   "func (f *FlagSet) Output() io.Writer",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Parse",
											Synopsis:   "func (f *FlagSet) Parse(arguments []string) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Parsed",
											Synopsis:   "func (f *FlagSet) Parsed() bool",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.PrintDefaults",
											Synopsis:   "func (f *FlagSet) PrintDefaults()",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Set",
											Synopsis:   "func (f *FlagSet) Set(name, value string) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.SetOutput",
											Synopsis:   "func (f *FlagSet) SetOutput(output io.Writer)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.String",
											Synopsis:   "func (f *FlagSet) String(name string, value string, usage string) *string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.StringVar",
											Synopsis:   "func (f *FlagSet) StringVar(p *string, name string, value string, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Uint",
											Synopsis:   "func (f *FlagSet) Uint(name string, value uint, usage string) *uint",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Uint64",
											Synopsis:   "func (f *FlagSet) Uint64(name string, value uint64, usage string) *uint64",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Uint64Var",
											Synopsis:   "func (f *FlagSet) Uint64Var(p *uint64, name string, value uint64, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.UintVar",
											Synopsis:   "func (f *FlagSet) UintVar(p *uint, name string, value uint, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Var",
											Synopsis:   "func (f *FlagSet) Var(value Value, name string, usage string)",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.Visit",
											Synopsis:   "func (f *FlagSet) Visit(fn func(*Flag))",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
										{
											Name:       "FlagSet.VisitAll",
											Synopsis:   "func (f *FlagSet) VisitAll(fn func(*Flag))",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "FlagSet",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Getter",
										Synopsis: "type Getter interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Getter.Get",
											Synopsis:   "Get func() interface{}",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Getter",
										},
									},
								},
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Value",
										Synopsis: "type Value interface{ ... }",
										Section:  "Types",
										Kind:     "Type",
									},
									Children: []*internal.SymbolMeta{
										{
											Name:       "Value.String",
											Synopsis:   "String func() string",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Value",
										},
										{
											Name:       "Value.Set",
											Synopsis:   "Set func(string) error",
											Section:    "Types",
											Kind:       "Method",
											ParentName: "Value",
										},
									},
								},
							},
						},
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
					Documentation: []*internal.Documentation{
						{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "package foo exports a helpful constant.",
							API: []*internal.Symbol{
								{
									SymbolMeta: internal.SymbolMeta{
										Name:     "Bar",
										Synopsis: "const Bar",
										Section:  "Constants",
										Kind:     "Constant",
									},
								},
							},
						},
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
					Documentation: []*internal.Documentation{{
						GOOS:     internal.All,
						GOARCH:   internal.All,
						Synopsis: "package foo exports a helpful constant.",
						API: []*internal.Symbol{
							{
								SymbolMeta: internal.SymbolMeta{
									Name:     "Bar",
									Synopsis: "const Bar",
									Section:  "Constants",
									Kind:     "Constant",
								},
							},
						},
					},
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
func moduleWithExamples(path string, api []*internal.Symbol, source, test string, docSubstrings ...string) *testModule {
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
			HasGoMod:  false,
			Module: &internal.Module{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        path,
					HasGoMod:          false,
					IsRedistributable: true,
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
						Documentation: []*internal.Documentation{{
							GOOS:     internal.All,
							GOARCH:   internal.All,
							Synopsis: "Package example contains examples.",
							API:      api,
						}},
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
	nil,
	``,
	`import "fmt"

// Example for the package.
func Example() {
	fmt.Println("hello")
	// Output: hello
}
`, "Documentation-exampleButtonsContainer")

var moduleFuncExample = moduleWithExamples("func.example",
	[]*internal.Symbol{
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "F",
				Synopsis: "func F()",
				Section:  "Functions",
				Kind:     "Function",
			},
		},
	},
	`func F() {}
`, `import "func.example/example"

// Example for the function.
func ExampleF() {
	example.F()
}
`, "Documentation-exampleButtonsContainer")

var moduleTypeExample = moduleWithExamples("type.example",
	[]*internal.Symbol{
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "T",
				Synopsis: "type T struct{}",
				Section:  "Types",
				Kind:     "Type",
			},
		},
	},

	`type T struct{}
`, `import "type.example/example"

// Example for the type.
func ExampleT() {
	example.T{}
}
`, "Documentation-exampleButtonsContainer")

var moduleMethodExample = moduleWithExamples("method.example",
	[]*internal.Symbol{
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "T",
				Synopsis: "type T struct{}",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
				{
					Name:       "T.M",
					Synopsis:   "func (*T) M()",
					Section:    "Types",
					Kind:       "Method",
					ParentName: "T",
				},
			},
		},
	},
	`type T struct {}

func (*T) M() {}
`, `import "method.example/example"

// Example for the method.
func ExampleT_M() {
	new(example.T).M()
}
`, "Documentation-exampleButtonsContainer")
