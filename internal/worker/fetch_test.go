// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
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
				"go.mod":        "module github.com/my/module\n\ngo 1.12",
				"LICENSE":       testhelper.BSD0License,
				"bar/README.md": "README FILE FOR TESTING.",
				"bar/LICENSE":   testhelper.MITLicense,
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

	myModuleV100 := &internal.Unit{
		UnitMeta: internal.UnitMeta{
			ModulePath:        "github.com/my/module",
			HasGoMod:          true,
			Version:           sample.VersionString,
			CommitTime:        testProxyCommitTime,
			SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", sample.VersionString),
			IsRedistributable: true,
			Path:              "github.com/my/module/bar",
			Name:              "bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
			},
		},
		Documentation: &internal.Documentation{
			Synopsis: "package bar",
			GOOS:     "linux",
			GOARCH:   "amd64",
		},
		Readme: &internal.Readme{
			Filepath: "bar/README.md",
			Contents: "README FILE FOR TESTING.",
		},
	}

	testCases := []struct {
		modulePath  string
		version     string
		pkg         string
		want        *internal.Unit
		wantDoc     []string // Substrings we expect to see in DocumentationHTML.
		dontWantDoc []string // Substrings we expect not to see in DocumentationHTML.
	}{
		{
			modulePath: "github.com/my/module",
			version:    sample.VersionString,
			pkg:        "github.com/my/module/bar",
			want:       myModuleV100,
			wantDoc:    []string{"Bar returns the string &#34;bar&#34;."},
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
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        "nonredistributable.mod/module",
					Version:           "v1.0.0",
					HasGoMod:          true,
					CommitTime:        testProxyCommitTime,
					SourceInfo:        nil,
					IsRedistributable: true,
					Path:              "nonredistributable.mod/module/bar/baz",
					Name:              "baz",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/baz/COPYING"},
					},
				},
				Documentation: &internal.Documentation{
					Synopsis: "package baz",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
			wantDoc: []string{"Baz returns the string &#34;baz&#34;."},
		}, {
			modulePath: "nonredistributable.mod/module",
			version:    sample.VersionString,
			pkg:        "nonredistributable.mod/module/foo",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        "nonredistributable.mod/module",
					Version:           sample.VersionString,
					HasGoMod:          true,
					CommitTime:        testProxyCommitTime,
					SourceInfo:        nil,
					IsRedistributable: false,
					Path:              "nonredistributable.mod/module/foo",
					Name:              "foo",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"UNKNOWN"}, FilePath: "foo/LICENSE.md"},
					},
				},
				NumImports: 2,
			},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "context",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        "std",
					Version:           "v1.12.5",
					HasGoMod:          true,
					CommitTime:        stdlib.TestCommitTime,
					SourceInfo:        source.NewStdlibInfo("v1.12.5"),
					IsRedistributable: true,
					Path:              "context",
					Name:              "context",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
				},
				NumImports: 5,
				Documentation: &internal.Documentation{
					Synopsis: "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
			wantDoc: []string{"This example demonstrates the use of a cancelable context to prevent a\ngoroutine leak."},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "builtin",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        "std",
					Version:           "v1.12.5",
					HasGoMod:          true,
					CommitTime:        stdlib.TestCommitTime,
					SourceInfo:        source.NewStdlibInfo("v1.12.5"),
					IsRedistributable: true,
					Path:              "builtin",
					Name:              "builtin",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
				},
				Documentation: &internal.Documentation{
					Synopsis: "Package builtin provides documentation for Go's predeclared identifiers.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
			wantDoc: []string{"int64 is the set of all signed 64-bit integers."},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "encoding/json",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        "std",
					Version:           "v1.12.5",
					HasGoMod:          true,
					CommitTime:        stdlib.TestCommitTime,
					SourceInfo:        source.NewStdlibInfo("v1.12.5"),
					IsRedistributable: true,
					Path:              "encoding/json",
					Name:              "json",
					Licenses: []*licenses.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
				},
				NumImports: 15,
				Documentation: &internal.Documentation{
					Synopsis: "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
			wantDoc: []string{
				"The mapping between JSON and Go values is described\nin the documentation for the Marshal and Unmarshal functions.",
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
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModulePath:        buildConstraintsMod.ModulePath,
					Version:           buildConstraintsMod.Version,
					HasGoMod:          false,
					CommitTime:        testProxyCommitTime,
					IsRedistributable: true,
					Path:              buildConstraintsMod.ModulePath + "/cpu",
					Name:              "cpu",
					Licenses: []*licenses.Metadata{
						{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE"},
					},
				},
				Documentation: &internal.Documentation{
					Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
			wantDoc: []string{"const CacheLinePadSize = 3"},
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

			if _, _, err := FetchAndUpdateState(ctx, test.modulePath, test.version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
				t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", test.modulePath, test.version, proxyClient, sourceClient, testDB, err)
			}

			got, err := testDB.GetUnitMeta(ctx, test.pkg, test.modulePath, test.want.Version)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(got.Licenses, func(i, j int) bool {
				return got.Licenses[i].FilePath < got.Licenses[j].FilePath
			})
			if diff := cmp.Diff(test.want.UnitMeta, *got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Fatalf("testDB.GetUnitMeta(ctx, %q, %q) mismatch (-want +got):\n%s", test.modulePath, test.version, diff)
			}

			gotPkg, err := testDB.GetUnit(ctx, got, internal.WithMain)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, gotPkg,
				cmp.AllowUnexported(source.Info{}),
				cmpopts.IgnoreFields(internal.Unit{}, "Documentation"),
				cmpopts.IgnoreFields(internal.Unit{}, "Subdirectories")); diff != "" {
				t.Errorf("mismatch on readme (-want +got):\n%s", diff)
			}
			if got, want := gotPkg.Documentation, test.want.Documentation; got == nil || want == nil {
				if got != want {
					t.Fatalf("mismatch on documentation: got: %v\nwant: %v", got, want)
				}
				return
			}
			if gotPkg.Documentation != nil {
				parts, err := godoc.RenderPartsFromUnit(ctx, gotPkg)
				if err != nil {
					t.Fatal(err)
				}
				gotDoc := parts.Body.String()
				for _, want := range test.wantDoc {
					if !strings.Contains(gotDoc, want) {
						t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", gotDoc, want)
					}
				}
				for _, dontWant := range test.dontWantDoc {
					if strings.Contains(gotDoc, dontWant) {
						t.Errorf("got documentation contains unwanted documentation substring:\ngot: %q\ndontWant (substring): %q", gotDoc, dontWant)
					}
				}
			}
		})
	}
}
