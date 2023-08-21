// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"fmt"
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
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/pkgsite/internal/version"
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

	defer stdlib.WithTestData()()

	proxyClient, teardownProxy := proxytest.SetupTestClient(t, testModules)
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
			version:    version.Latest,
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
	for _, test := range testCases {
		t.Run(strings.ReplaceAll(test.pkg+"@"+test.version, "/", " "), func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)
			f := &Fetcher{
				ProxyClient:  proxyClient.WithCache(),
				SourceClient: sourceClient,
				DB:           testDB,
				Cache:        nil,
				loadShedder:  &loadShedder{maxSizeInFlight: 100 * mib},
			}
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
			if diff := cmp.Diff(test.want.UnitMeta, *got, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Fatalf("testDB.GetUnitMeta(ctx, %q, %q) mismatch (-want +got):\n%s", test.modulePath, test.version, diff)
			}

			gotPkg, err := testDB.GetUnit(ctx, got, internal.WithMain, internal.BuildContext{})
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, gotPkg,
				cmpopts.EquateEmpty(),
				cmp.AllowUnexported(source.Info{}),
				cmpopts.IgnoreFields(internal.Unit{}, "Documentation", "BuildContexts"),
				cmpopts.IgnoreFields(internal.Unit{}, "SymbolHistory"),
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
				parts, err := godoc.RenderFromUnit(ctx, gotPkg, internal.BuildContext{})
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

func TestFetchAndUpdateStateCacheZip(t *testing.T) {
	// We can try to download a zip from the proxy twice when we are processing
	// a new module at the latest compatible version, and there is an
	// incompatible version. In that case, fetch.LatestModuleVersions needs to
	// download the zip to see if there is a go.mod file, and then the zip is
	// downloaded again in fetch.FetchModule. To avoid the double download, the
	// proxy can be set up with a small cache for the last downloaded zip. This
	// test confirms that that feature works.
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)
	proxyServer := proxytest.NewServer([]*proxytest.Module{
		{
			ModulePath: "m.com",
			Version:    "v2.0.0+incompatible",
			Files:      map[string]string{"a.go": "package a"},
		},
		{
			ModulePath: "m.com",
			Version:    "v1.0.0",
			Files:      map[string]string{"a.go": "package a"},
		},
	})
	proxyClient, teardownProxy, err := proxytest.NewClientForServer(proxyServer)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownProxy()

	// With a plain proxy, we download the zip twice.
	f := &Fetcher{proxyClient, source.NewClient(sourceTimeout), testDB, nil, nil, ""}
	if _, _, err := f.FetchAndUpdateState(ctx, "m.com", "v1.0.0", testAppVersion); err != nil {
		t.Fatal(err)
	}
	if got, want := proxyServer.ZipRequests(), 2; got != want {
		t.Errorf("got %d downloads, want %d", got, want)
	}

	// With the cache, we download it only once.
	postgres.ResetTestDB(testDB, t) // to avoid finding has_go_mod in the DB
	f.ProxyClient = proxyClient.WithCache()
	if _, _, err := f.FetchAndUpdateState(ctx, "m.com", "v1.0.0", testAppVersion); err != nil {
		t.Fatal(err)
	}
	// We want three total zip requests: 2 before, 1 now.
	if got, want := proxyServer.ZipRequests(), 3; got != want {
		t.Errorf("got %d downloads, want %d", got, want)
	}

}

func TestFetchAndUpdateLatest(t *testing.T) {
	ctx := context.Background()
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	const modulePath = "example.com/retractions"
	f := &Fetcher{
		ProxyClient:  prox,
		SourceClient: source.NewClient(sourceTimeout),
		DB:           testDB,
	}
	got, err := f.FetchAndUpdateLatest(ctx, modulePath)
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

func TestFetchGo121(t *testing.T) {
	// This test verifies that we can fetch modules using the more relaxed go
	// directive syntax added with Go 1.21 (e.g. `go 1.21.0`).
	var (
		modulePath = sample.ModulePath
		version    = sample.VersionString
		foo        = map[string]string{
			"go.mod":     fmt.Sprintf("module %s\n\ngo 1.21.0\n", modulePath),
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
	)
	proxyClient, teardownProxy := proxytest.SetupTestClient(t, []*proxytest.Module{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()

	sourceClient := source.NewClient(sourceTimeout)
	f := &Fetcher{proxyClient, sourceClient, testDB, nil, nil, ""}
	got, _, err := f.FetchAndUpdateState(context.Background(), modulePath, version, testAppVersion)
	if err != nil {
		t.Fatalf("FetchAndUpdateState(%q, %q): %v", sample.ModulePath, version, err)
	}
	want := 200
	if got != want {
		t.Fatalf("FetchAndUpdateState(%q, %q): status = %d, want %d", sample.ModulePath, version, got, want)
	}
}
