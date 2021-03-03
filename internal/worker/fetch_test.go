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
)

const (
	// Indicates that although we have a valid module, some packages could not be processed.
	hasIncompletePackagesCode = 290
	hasIncompletePackagesDesc = "incomplete packages"

	testAppVersion             = "appVersionLabel"
	buildConstraintsModulePath = "example.com/build-constraints"
	buildConstraintsVersion    = "v1.0.0"
)

var (
	sourceTimeout       = 1 * time.Second
	testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
)

func TestFetchAndUpdateState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	stdlib.UseTestData = true
	defer func() { stdlib.UseTestData = false }()

	proxyClient, teardownProxy := proxy.SetupTestClient(t, testModules)
	defer teardownProxy()

	myModuleV100 := &internal.Unit{
		UnitMeta: internal.UnitMeta{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "example.com/multi",
				HasGoMod:          true,
				Version:           sample.VersionString,
				CommitTime:        testProxyCommitTime,
				SourceInfo:        source.NewGitHubInfo("https://example.com/multi", "", sample.VersionString),
				IsRedistributable: true,
			},
			IsRedistributable: true,
			Path:              "example.com/multi/bar",
			Name:              "bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"0BSD"}, FilePath: "LICENSE"},
				{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
			},
		},
		Documentation: []*internal.Documentation{{
			Synopsis: "package bar",
			GOOS:     "linux",
			GOARCH:   "amd64",
		}},
		Readme: &internal.Readme{
			Filepath: "bar/README",
			Contents: "Another README file for testing.",
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
			modulePath: "example.com/multi",
			version:    sample.VersionString,
			pkg:        "example.com/multi/bar",
			want:       myModuleV100,
			wantDoc:    []string{"Bar returns the string &#34;bar&#34;."},
		},
		{
			modulePath: "example.com/multi",
			version:    internal.LatestVersion,
			pkg:        "example.com/multi/bar",
			want:       myModuleV100,
		},
		{
			// example.com/nonredist is redistributable, as are its
			// packages bar and bar/baz. But package unk is not.
			modulePath: "example.com/nonredist",
			version:    sample.VersionString,
			pkg:        "example.com/nonredist/bar/baz",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "example.com/nonredist",
						Version:           sample.VersionString,
						HasGoMod:          true,
						CommitTime:        testProxyCommitTime,
						SourceInfo:        source.NewGitHubInfo("https://example.com/nonredist", "", sample.VersionString),
						IsRedistributable: true,
					},
					IsRedistributable: true,
					Path:              "example.com/nonredist/bar/baz",
					Name:              "baz",
					Licenses: []*licenses.Metadata{
						{Types: []string{"0BSD"}, FilePath: "LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/baz/COPYING"},
					},
				},
				Documentation: []*internal.Documentation{{
					Synopsis: "package baz",
					GOOS:     "linux",
					GOARCH:   "amd64",
				}},
			},
			wantDoc: []string{"Baz returns the string &#34;baz&#34;."},
		}, {
			modulePath: "example.com/nonredist",
			version:    sample.VersionString,
			pkg:        "example.com/nonredist/unk",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "example.com/nonredist",
						Version:           sample.VersionString,
						HasGoMod:          true,
						CommitTime:        testProxyCommitTime,
						SourceInfo:        source.NewGitHubInfo("https://example.com/nonredist", "", sample.VersionString),
						IsRedistributable: true,
					},
					IsRedistributable: false,
					Path:              "example.com/nonredist/unk",
					Name:              "unk",
					Licenses: []*licenses.Metadata{
						{Types: []string{"0BSD"}, FilePath: "LICENSE"},
						{Types: []string{"UNKNOWN"}, FilePath: "unk/LICENSE.md"},
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
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						HasGoMod:          true,
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewStdlibInfo("v1.12.5"),
						IsRedistributable: true,
					},
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
				Documentation: []*internal.Documentation{{
					Synopsis: "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				}},
			},
			wantDoc: []string{"This example demonstrates the use of a cancelable context to prevent a\ngoroutine leak."},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "builtin",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						HasGoMod:          true,
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewStdlibInfo("v1.12.5"),
						IsRedistributable: true,
					},
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
				Documentation: []*internal.Documentation{{
					Synopsis: "Package builtin provides documentation for Go's predeclared identifiers.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				}},
			},
			wantDoc: []string{"int64 is the set of all signed 64-bit integers."},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "encoding/json",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        "std",
						Version:           "v1.12.5",
						HasGoMod:          true,
						CommitTime:        stdlib.TestCommitTime,
						SourceInfo:        source.NewStdlibInfo("v1.12.5"),
						IsRedistributable: true,
					},
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
				Documentation: []*internal.Documentation{{
					Synopsis: "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				}},
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
			modulePath: buildConstraintsModulePath,
			version:    sample.VersionString,
			pkg:        buildConstraintsModulePath + "/cpu",
			want: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						ModulePath:        buildConstraintsModulePath,
						Version:           "v1.0.0",
						HasGoMod:          true,
						CommitTime:        testProxyCommitTime,
						SourceInfo:        source.NewGitHubInfo("https://"+buildConstraintsModulePath, "", sample.VersionString),
						IsRedistributable: true,
					},
					IsRedistributable: true,
					Path:              buildConstraintsModulePath + "/cpu",
					Name:              "cpu",
					Licenses: []*licenses.Metadata{
						{Types: []string{"0BSD"}, FilePath: "LICENSE"},
					},
				},
				Documentation: []*internal.Documentation{{
					Synopsis: "Package cpu implements processor feature detection used by the Go standard library.",
					GOOS:     "linux",
					GOARCH:   "amd64",
				}},
			},
			wantDoc: []string{"const CacheLinePadSize = 3"},
			dontWantDoc: []string{
				"const CacheLinePadSize = 1",
				"const CacheLinePadSize = 2",
			},
		},
	}

	sourceClient := source.NewClient(sourceTimeout)
	f := &Fetcher{proxyClient, sourceClient, testDB, nil}
	for _, test := range testCases {
		t.Run(strings.ReplaceAll(test.pkg+"@"+test.version, "/", " "), func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)
			if _, _, err := f.FetchAndUpdateState(ctx, test.modulePath, test.version, testAppVersion); err != nil {
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
				if !cmp.Equal(got, want) {
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
			// TODO(https://golang.org/issue/43890): fix 500 error for
			// fetching std@master and update test.
			if test.modulePath != stdlib.ModulePath {
				for _, v := range []string{internal.MainVersion, internal.MasterVersion, test.version} {
					if _, err := testDB.GetVersionMap(ctx, test.modulePath, v); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
}

func TestFetchAndUpdateLatest(t *testing.T) {
	ctx := context.Background()
	prox, teardown := proxy.SetupTestClient(t, testModules)
	defer teardown()

	const modulePath = "example.com/retractions"
	f := &Fetcher{
		ProxyClient:  prox,
		SourceClient: source.NewClient(sourceTimeout),
		DB:           testDB,
	}
	if err := f.fetchAndUpdateLatest(ctx, modulePath); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetLatestModuleVersions(ctx, modulePath)
	if err != nil {
		t.Fatal(err)
	}
	const (
		wantRaw    = "v1.2.0"
		wantCooked = "v1.0.0"
	)
	if got.ModulePath != modulePath || got.RawVersion != wantRaw || got.CookedVersion != wantCooked {
		t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
			got.ModulePath, got.RawVersion, got.CookedVersion,
			modulePath, wantRaw, wantCooked)
	}
}
