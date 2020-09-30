// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

const (
	// Indicates that although we have a valid module, some packages could not be processed.
	hasIncompletePackagesCode = 290
	hasIncompletePackagesDesc = "incomplete packages"

	testAppVersion = "appVersionLabel"
)

var (
	sourceTimeout       = 1 * time.Second
	testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
)

var buildConstraintsMod = &proxy.Module{
	ModulePath: "build.constraints/module",
	Version:    sample.VersionString,
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
}

var html = testconversions.MakeHTMLForTest

func TestFetch_V1Path(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	proxyClient, tearDown := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Files: map[string]string{
				"foo.go":  "package foo\nconst Foo = 41",
				"LICENSE": testhelper.MITLicense,
			},
		},
	})
	defer tearDown()
	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusOK)
	pkg, err := testDB.LegacyGetPackage(ctx, sample.ModulePath, internal.UnknownModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pkg.V1Path, sample.ModulePath; got != want {
		t.Errorf("V1Path = %q, want %q", got, want)
	}
}

func TestFetchAndUpdateState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const goRepositoryURLPrefix = "https://github.com/golang"

	stdlib.UseTestData = true
	defer func() { stdlib.UseTestData = false }()

	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		buildConstraintsMod,
		{
			ModulePath: "github.com/my/module",
			Files: map[string]string{
				"go.mod":      "module github.com/my/module\n\ngo 1.12",
				"LICENSE":     testhelper.BSD0License,
				"README.md":   "README FILE FOR TESTING.",
				"bar/LICENSE": testhelper.MITLicense,
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

		{
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
	})
	defer teardownProxy()

	myModuleV100 := &internal.LegacyVersionedPackage{
		LegacyModuleInfo: internal.LegacyModuleInfo{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "github.com/my/module",
				Version:    sample.VersionString,
				CommitTime: testProxyCommitTime,
				SourceInfo: source.NewGitHubInfo("https://github.com/my/module", "", sample.VersionString),

				IsRedistributable: true,
				HasGoMod:          true,
			},
			LegacyReadmeFilePath: "README.md",
			LegacyReadmeContents: "README FILE FOR TESTING.",
		},
		LegacyPackage: internal.LegacyPackage{
			Path:              "github.com/my/module/bar",
			Name:              "bar",
			Synopsis:          "package bar",
			DocumentationHTML: html("Bar returns the string &#34;bar&#34;."),
			V1Path:            "github.com/my/module/bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
			},
			IsRedistributable: true,
			GOOS:              "linux",
			GOARCH:            "amd64",
		},
	}

	testCases := []struct {
		modulePath  string
		version     string
		pkg         string
		want        *internal.LegacyVersionedPackage
		moreWantDoc []string // Additional substrings we expect to see in DocumentationHTML.
		dontWantDoc []string // Substrings we expect not to see in DocumentationHTML.
	}{
		{
			modulePath: "github.com/my/module",
			version:    sample.VersionString,
			pkg:        "github.com/my/module/bar",
			want:       myModuleV100,
		},
		{
			modulePath: "github.com/my/module",
			version:    internal.LatestVersion,
			pkg:        "github.com/my/module/bar",
			want:       myModuleV100,
		},
		{
			// nonredistributable.mod/module is redistributable, as are its
			// packages bar and bar/baz. But package foo is not.
			modulePath: "nonredistributable.mod/module",
			version:    sample.VersionString,
			pkg:        "nonredistributable.mod/module/bar/baz",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "nonredistributable.mod/module",
						Version:           "v1.0.0",
						CommitTime:        testProxyCommitTime,
						SourceInfo:        nil,
						IsRedistributable: true,
						HasGoMod:          true,
					},
					LegacyReadmeFilePath: "README.md",
					LegacyReadmeContents: "README FILE FOR TESTING.",
				},
				LegacyPackage: internal.LegacyPackage{
					Path:              "nonredistributable.mod/module/bar/baz",
					Name:              "baz",
					Synopsis:          "package baz",
					DocumentationHTML: html("Baz returns the string &#34;baz&#34;."),
					V1Path:            "nonredistributable.mod/module/bar/baz",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/baz/COPYING"},
					},
					IsRedistributable: true,
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		}, {
			modulePath: "nonredistributable.mod/module",
			version:    sample.VersionString,
			pkg:        "nonredistributable.mod/module/foo",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "nonredistributable.mod/module",
						Version:           sample.VersionString,
						CommitTime:        testProxyCommitTime,
						SourceInfo:        nil,
						IsRedistributable: true,
						HasGoMod:          true,
					},
					LegacyReadmeFilePath: "README.md",
					LegacyReadmeContents: "README FILE FOR TESTING.",
				},
				LegacyPackage: internal.LegacyPackage{
					Path:     "nonredistributable.mod/module/foo",
					Name:     "foo",
					Synopsis: "",
					V1Path:   "nonredistributable.mod/module/foo",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"UNKNOWN"}, FilePath: "foo/LICENSE.md"},
					},
					GOOS:              "linux",
					GOARCH:            "amd64",
					IsRedistributable: false,
				},
			},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "context",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
						IsRedistributable: true,
						HasGoMod:          true,
					},
					LegacyReadmeFilePath: "README.md",
					LegacyReadmeContents: "# The Go Programming Language\n",
				},
				LegacyPackage: internal.LegacyPackage{
					Path:              "context",
					Name:              "context",
					Synopsis:          "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					DocumentationHTML: html("This example demonstrates the use of a cancelable context to prevent a\ngoroutine leak."),
					V1Path:            "context",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
					IsRedistributable: true,
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "builtin",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
						IsRedistributable: true,
						HasGoMod:          true,
					},

					LegacyReadmeFilePath: "README.md",
					LegacyReadmeContents: "# The Go Programming Language\n",
				},
				LegacyPackage: internal.LegacyPackage{
					Path:              "builtin",
					Name:              "builtin",
					Synopsis:          "Package builtin provides documentation for Go's predeclared identifiers.",
					DocumentationHTML: html("int64 is the set of all signed 64-bit integers."),
					V1Path:            "builtin",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
					IsRedistributable: true,
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "encoding/json",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
						IsRedistributable: true,
						HasGoMod:          true,
					},
					LegacyReadmeFilePath: "README.md",
					LegacyReadmeContents: "# The Go Programming Language\n",
				},
				LegacyPackage: internal.LegacyPackage{
					Path:              "encoding/json",
					Name:              "json",
					Synopsis:          "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
					DocumentationHTML: html("The mapping between JSON and Go values is described\nin the documentation for the Marshal and Unmarshal functions."),
					V1Path:            "encoding/json",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
					IsRedistributable: true,
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
			moreWantDoc: []string{
				"Example (CustomMarshalJSON)",
				`<summary class="Documentation-exampleDetailsHeader">Example (CustomMarshalJSON) <a href="#example-package-CustomMarshalJSON">¶</a></summary>`,
				"Package (CustomMarshalJSON)",
				`<li><a href="#example-package-CustomMarshalJSON" class="js-exampleHref">Package (CustomMarshalJSON)</a></li>`,
				"Decoder.Decode (Stream)",
				`<li><a href="#example-Decoder.Decode-Stream" class="js-exampleHref">Decoder.Decode (Stream)</a></li>`,
			},
			dontWantDoc: []string{
				"Example (customMarshalJSON)",
				"Package (customMarshalJSON)",
				"Decoder.Decode (stream)",
			},
		}, {
			modulePath: buildConstraintsMod.ModulePath,
			version:    buildConstraintsMod.Version,
			pkg:        buildConstraintsMod.ModulePath + "/cpu",
			want: &internal.LegacyVersionedPackage{
				LegacyModuleInfo: internal.LegacyModuleInfo{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        buildConstraintsMod.ModulePath,
						Version:           buildConstraintsMod.Version,
						CommitTime:        testProxyCommitTime,
						IsRedistributable: true,
						HasGoMod:          false,
					},
				},
				LegacyPackage: internal.LegacyPackage{
					Path:              buildConstraintsMod.ModulePath + "/cpu",
					Name:              "cpu",
					Synopsis:          "Package cpu implements processor feature detection used by the Go standard library.",
					DocumentationHTML: html("const CacheLinePadSize = 3"),
					V1Path:            buildConstraintsMod.ModulePath + "/cpu",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
					},
					IsRedistributable: true,
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
			dontWantDoc: []string{
				"const CacheLinePadSize = 1",
				"const CacheLinePadSize = 2",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.pkg, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			sourceClient := source.NewClient(sourceTimeout)

			if _, err := FetchAndUpdateState(ctx, test.modulePath, test.version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
				t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", test.modulePath, test.version, proxyClient, sourceClient, testDB, err)
			}

			gotModuleInfo, err := testDB.GetModuleInfo(ctx, test.modulePath, test.want.Version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want.ModuleInfo, *gotModuleInfo, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Fatalf("testDB.GetModuleInfo(ctx, %q, %q) mismatch (-want +got):\n%s", test.modulePath, test.version, diff)
			}

			gotPkg, err := testDB.LegacyGetPackage(ctx, test.pkg, internal.UnknownModulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}

			sort.Slice(gotPkg.Licenses, func(i, j int) bool {
				return gotPkg.Licenses[i].FilePath < gotPkg.Licenses[j].FilePath
			})
			if diff := cmp.Diff(test.want, gotPkg, cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.LegacyGetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkg, test.version, diff)
			}
			if got, want := gotPkg.DocumentationHTML.String(), test.want.DocumentationHTML.String(); len(want) == 0 && len(got) != 0 {
				t.Errorf("got non-empty documentation but want empty:\ngot: %q\nwant: %q", got, want)
			} else if !strings.Contains(got, want) {
				t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
			}
			for _, want := range test.moreWantDoc {
				if got := gotPkg.DocumentationHTML.String(); !strings.Contains(got, want) {
					t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
				}
			}
			for _, dontWant := range test.dontWantDoc {
				if got := gotPkg.DocumentationHTML.String(); strings.Contains(got, dontWant) {
					t.Errorf("got documentation contains unwanted documentation substring:\ngot: %q\ndontWant (substring): %q", got, dontWant)
				}
			}
		})
	}
}
