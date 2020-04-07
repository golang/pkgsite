// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"net/http"

	"github.com/google/licensecheck"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
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
				ModulePath:        "github.com/basic",
				ReadmeFilePath:    "README.md",
				ReadmeContents:    "THIS IS A README",
				IsRedistributable: true,
				HasGoMod:          false,
			},
			Packages: []*internal.Package{
				{
					Name:              "foo",
					Path:              "github.com/basic/foo",
					V1Path:            "github.com/basic/foo",
					Synopsis:          "package foo exports a helpful constant.",
					IsRedistributable: true,
					Imports:           []string{"net/http"},
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		},
	},
}

var moduleMultiPackage = &testModule{
	mod: &proxy.TestModule{
		ModulePath: "github.com/my/module",
		Files: map[string]string{
			"go.mod":      "module github.com/my/module\n\ngo 1.12",
			"LICENSE":     testhelper.BSD0License,
			"README.md":   "README FILE FOR TESTING.",
			"bar/COPYING": testhelper.MITLicense,
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
				ModulePath:        "github.com/my/module",
				IsRedistributable: true,
				HasGoMod:          true,
				ReadmeFilePath:    "README.md",
				ReadmeContents:    "README FILE FOR TESTING.",
				SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
			},
			Licenses: []*licenses.License{
				{
					Metadata: &licenses.Metadata{
						Types:    []string{"BSD-0-Clause"},
						FilePath: "LICENSE",
						Coverage: licensecheck.Coverage{
							Percent: 100,
							Match: []licensecheck.Match{
								{
									Name:    "BSD-0-Clause",
									Type:    licensecheck.BSD,
									Percent: 100,
								},
							},
						},
					},
					Contents: []byte(testhelper.BSD0License),
				},
				{
					Metadata: &licenses.Metadata{
						Types:    []string{"MIT"},
						FilePath: "bar/COPYING",
						Coverage: licensecheck.Coverage{
							Percent: 100,
							Match: []licensecheck.Match{
								{
									Name:    "MIT",
									Type:    licensecheck.MIT,
									Percent: 100,
								},
							},
						},
					},
					Contents: []byte(testhelper.MITLicense),
				},
				{
					Metadata: &licenses.Metadata{
						Types:    []string{"MIT"},
						FilePath: "foo/LICENSE.md",
						Coverage: licensecheck.Coverage{
							Percent: 100,
							Match: []licensecheck.Match{
								{
									Name:    "MIT",
									Type:    licensecheck.MIT,
									Percent: 100,
								},
							},
						},
					},
					Contents: []byte(testhelper.MITLicense),
				},
			},
			Packages: []*internal.Package{
				{
					Name:              "bar",
					Path:              "github.com/my/module/bar",
					Synopsis:          "package bar",
					DocumentationHTML: "Bar returns the string &#34;bar&#34;.",
					Imports:           []string{},
					V1Path:            "github.com/my/module/bar",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"MIT"},
							FilePath: "bar/COPYING",
							Coverage: licensecheck.Coverage{
								Percent: 100,
								Match:   []licensecheck.Match{{Name: "MIT", Type: licensecheck.MIT, Percent: 100}},
							},
						},
						{
							Types:    []string{"BSD-0-Clause"},
							FilePath: "LICENSE",
							Coverage: licensecheck.Coverage{
								Percent: 100,
								Match:   []licensecheck.Match{{Name: "BSD-0-Clause", Type: licensecheck.BSD, Percent: 100}},
							},
						},
					},
				},
				{
					Name:              "foo",
					Path:              "github.com/my/module/foo",
					Synopsis:          "package foo",
					DocumentationHTML: "FooBar returns the string &#34;foo bar&#34;.",
					Imports:           []string{"fmt", "github.com/my/module/bar"},
					V1Path:            "github.com/my/module/foo",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"MIT"},
							FilePath: "foo/LICENSE.md",
							Coverage: licensecheck.Coverage{
								Percent: 100,
								Match:   []licensecheck.Match{{Name: "MIT", Type: licensecheck.MIT, Percent: 100}},
							},
						},
						{
							Types:    []string{"BSD-0-Clause"},
							FilePath: "LICENSE",
							Coverage: licensecheck.Coverage{
								Percent: 100,
								Match:   []licensecheck.Match{{Name: "BSD-0-Clause", Type: licensecheck.BSD, Percent: 100}},
							},
						},
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
				ModulePath:        "no.mod/module",
				IsRedistributable: true,
				HasGoMod:          false,
			},
			Packages: []*internal.Package{
				{
					Path:              "no.mod/module/p",
					Name:              "p",
					Synopsis:          "Package p is inside a module where a go.mod file hasn't been explicitly added yet.",
					DocumentationHTML: "const Year = 2009",
					Imports:           []string{},
					V1Path:            "no.mod/module/p",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
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
				ModulePath:        "bad.mod/module",
				IsRedistributable: true,
			},
			Packages: []*internal.Package{
				{
					Name:              "good",
					Path:              "bad.mod/module/good",
					Synopsis:          "Package good is inside a module that has bad packages.",
					DocumentationHTML: `const Good = <a href="/pkg/builtin#true">true</a>`,
					Imports:           []string{},
					V1Path:            "bad.mod/module/good",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
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
				ModulePath:        "build.constraints/module",
				IsRedistributable: true,
				HasGoMod:          false,
			},
			Packages: []*internal.Package{
				{
					Name:              "cpu",
					Path:              "build.constraints/module/cpu",
					Synopsis:          "Package cpu implements processor feature detection used by the Go standard library.",
					DocumentationHTML: "const CacheLinePadSize = 3",
					Imports:           []string{},
					V1Path:            "build.constraints/module/cpu",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
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
			Licenses: []*licenses.License{},
			Packages: []*internal.Package{
				{
					Path:    "bad.import.path.com/good/import/path",
					Name:    "foo",
					V1Path:  "bad.import.path.com/good/import/path",
					Imports: []string{},
					GOOS:    "linux",
					GOARCH:  "amd64",
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
				ModulePath:        "doc.test",
				IsRedistributable: true,
				HasGoMod:          false,
			},
			Packages: []*internal.Package{
				{
					Path:              "doc.test/permalink",
					Name:              "permalink",
					Synopsis:          "Package permalink is for testing the heading permalink documentation rendering feature.",
					DocumentationHTML: "<h3 id=\"hdr-This_is_a_heading\">This is a heading <a href=\"#hdr-This_is_a_heading\">¶</a></h3>",
					Imports:           []string{},
					V1Path:            "doc.test/permalink",
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: true,
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
				ModulePath:        "github.com/my/module/js",
				IsRedistributable: true,
				ReadmeFilePath:    "README.md",
				ReadmeContents:    "THIS IS A README",
				SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "js", "js/v1.0.0"),
			},
			Packages: []*internal.Package{
				{
					Path:              "github.com/my/module/js/js",
					V1Path:            "github.com/my/module/js/js",
					Name:              "js",
					Synopsis:          "Package js only works with wasm.",
					IsRedistributable: true,
					Imports:           []string{},
					GOOS:              "js",
					GOARCH:            "wasm",
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
