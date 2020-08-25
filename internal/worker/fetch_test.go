// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

const testAppVersion = "appVersionLabel"

var sourceTimeout = 1 * time.Second

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

// Check that when the proxy says it does not have module@version,
// we delete it from the database.
func TestFetchAndUpdateState_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardown := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Files: map[string]string{
				"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
				"README.md":  "This is a readme",
				"LICENSE":    testhelper.MITLicense,
			},
		},
	})
	defer teardown()
	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusOK)

	// Take down the module, by having the proxy serve a 404/410 for it.
	proxyServer := proxy.NewServer([]*proxy.Module{})
	proxyServer.AddRoute(
		fmt.Sprintf("/%s/@v/%s.info", sample.ModulePath, sample.VersionString),
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "taken down", http.StatusGone) })
	proxyClient, teardownProxy2, err := proxy.NewClientForServer(proxyServer)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownProxy2()

	// Now fetch it again. The new state should have a status of Not Found.
	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusNotFound)
	checkPackageVersionStates(ctx, t, sample.ModulePath, sample.VersionString, []*internal.PackageVersionState{
		{
			PackagePath: sample.ModulePath + "/foo",
			ModulePath:  sample.ModulePath,
			Version:     sample.VersionString,
			Status:      http.StatusOK,
		},
	})

	// The module should no longer be in the database:
	// - It shouldn't be in the modules table. That also covers licenses, packages and paths tables
	//   via foreign key constraints with ON DELETE CASCADE.
	// - It shouldn't be in other tables like search_documents and the various imports tables.
	if _, err := testDB.GetModuleInfo(ctx, sample.ModulePath, sample.VersionString); !errors.Is(err, derrors.NotFound) {
		t.Fatalf("GetModuleInfo: got %v, want NotFound", err)
	}
	checkNotInTable := func(table, column string) {
		q := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1 LIMIT 1", table, column)
		var x int
		err := testDB.Underlying().QueryRow(ctx, q, sample.ModulePath).Scan(&x)
		if err != sql.ErrNoRows {
			t.Errorf("table %s: got %v, want ErrNoRows", table, err)
		}
	}
	checkNotInTable("search_documents", "module_path")
	checkNotInTable("imports_unique", "from_module_path")
	checkNotInTable("imports", "from_module_path")
}

func TestFetchAndUpdateState_Excluded(t *testing.T) {
	// Check that an excluded module is not processed, and is marked excluded in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestClient(t, nil)
	defer teardownProxy()

	if err := testDB.InsertExcludedPrefix(ctx, sample.ModulePath, "user", "for testing"); err != nil {
		t.Fatal(err)
	}

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusForbidden)
}

func TestFetchAndUpdateState_BadRequestedVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	var (
		modulePath = buildConstraintsMod.ModulePath
		version    = "badversion"
	)
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{buildConstraintsMod})
	defer teardownProxy()
	fetchAndCheckStatus(ctx, t, proxyClient, modulePath, version, http.StatusNotFound)
}

func TestFetchAndUpdateState_Incomplete(t *testing.T) {
	// Check that we store the special "incomplete" status in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{buildConstraintsMod})
	defer teardownProxy()

	fetchAndCheckStatus(ctx, t, proxyClient, buildConstraintsMod.ModulePath, buildConstraintsMod.Version, hasIncompletePackagesCode)
	checkPackageVersionStates(ctx, t, buildConstraintsMod.ModulePath, buildConstraintsMod.Version, []*internal.PackageVersionState{
		{
			PackagePath: buildConstraintsMod.ModulePath + "/cpu",
			ModulePath:  buildConstraintsMod.ModulePath,
			Version:     buildConstraintsMod.Version,
			Status:      200,
		},
		{
			PackagePath: buildConstraintsMod.ModulePath + "/ignore",
			ModulePath:  buildConstraintsMod.ModulePath,
			Version:     buildConstraintsMod.Version,
			Status:      600,
		},
	})
}

func TestFetchAndUpdateState_Mismatch(t *testing.T) {
	// Check that an excluded module is not processed, and is marked excluded in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	const goModPath = "other"
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Files: map[string]string{
				"go.mod":     "module " + goModPath,
				"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			},
		},
	})
	defer teardownProxy()

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString,
		derrors.ToStatus(derrors.AlternativeModule))
}

func TestFetchAndUpdateState_DeleteOlder(t *testing.T) {
	// Check that fetching an alternative module deletes all older versions of that
	// module from search_documents (but not versions).
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	const (
		mismatchVersion = "v1.0.0"
		olderVersion    = "v0.9.0"
	)

	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			// mismatched version; will cause deletion
			ModulePath: sample.ModulePath,
			Version:    mismatchVersion,
			Files: map[string]string{
				"go.mod":     "module other",
				"foo/foo.go": "package foo",
			},
		},
		{
			// older version; should be deleted
			ModulePath: sample.ModulePath,
			Version:    olderVersion,
			Files: map[string]string{
				"foo/foo.go": "package foo",
			},
		},
	})
	defer teardownProxy()

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, olderVersion, http.StatusOK)
	gotModule, gotVersion, gotFound := postgres.GetFromSearchDocuments(ctx, t, testDB, sample.ModulePath+"/foo")
	if !gotFound || gotModule != sample.ModulePath || gotVersion != olderVersion {
		t.Fatalf("got (%q, %q, %t), want (%q, %q, true)", gotModule, gotVersion, gotFound, sample.ModulePath, olderVersion)
	}

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, mismatchVersion, derrors.ToStatus(derrors.AlternativeModule))
	if _, _, gotFound := postgres.GetFromSearchDocuments(ctx, t, testDB, sample.ModulePath+"/foo"); gotFound {
		t.Fatal("older version found in search documents")
	}
}

func TestFetchAndUpdateState_SkipIncompletePackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	badModule := map[string]string{
		"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
		"README.md":  "This is a readme",
		"LICENSE":    testhelper.MITLicense,
	}
	var bigFile strings.Builder
	bigFile.WriteString("package bar\n")
	bigFile.WriteString("const Bar = 123\n")
	for bigFile.Len() <= fetch.MaxFileSize {
		bigFile.WriteString("// All work and no play makes Jack a dull boy.\n")
	}
	badModule["bar/bar.go"] = bigFile.String()
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Files:      badModule,
		},
	})
	defer teardownProxy()
	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, hasIncompletePackagesCode)
	checkPackage(ctx, t, sample.ModulePath+"/foo")
	if _, err := testDB.LegacyGetPackage(ctx, sample.ModulePath+"/bar", internal.UnknownModulePath, sample.VersionString); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}
}

// Test that large string literals and slices are trimmed when
// rendering documentation, rather than being included verbatim.
//
// This makes it viable for us to show documentation for packages that
// would otherwise exceed HTML size limit and not get shown at all.
func TestTrimLargeCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	trimmedModule := map[string]string{
		"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
		"LICENSE":    testhelper.MITLicense,
	}
	// Create a package with a large string literal. It should not be included verbatim.
	{
		var b strings.Builder
		b.WriteString("package bar\n\n")
		b.WriteString("const Bar = `\n")
		for b.Len() <= fetch.MaxDocumentationHTML {
			b.WriteString("All work and no play makes Jack a dull boy.\n")
		}
		b.WriteString("`\n")
		trimmedModule["bar/bar.go"] = b.String()
	}
	// Create a package with a large slice. It should not be included verbatim.
	{
		var b strings.Builder
		b.WriteString("package baz\n\n")
		b.WriteString("var Baz = []string{\n")
		for b.Len() <= fetch.MaxDocumentationHTML {
			b.WriteString("`All work and no play makes Jack a dull boy.`,\n")
		}
		b.WriteString("}\n")
		trimmedModule["baz/baz.go"] = b.String()
	}
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Files:      trimmedModule,
		},
	})
	defer teardownProxy()

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusOK)
	checkPackage(ctx, t, sample.ModulePath+"/foo")
	checkPackage(ctx, t, sample.ModulePath+"/bar")
	checkPackage(ctx, t, sample.ModulePath+"/baz")
}

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

func TestReFetch(t *testing.T) {
	// This test checks that re-fetching a version will cause its data to be
	// overwritten.  This is achieved by fetching against two different versions
	// of the (fake) proxy, though in reality the most likely cause of changes to
	// a version is updates to our data model or fetch logic.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	var (
		modulePath = sample.ModulePath
		version    = sample.VersionString
		pkgFoo     = sample.ModulePath + "/foo"
		foo        = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
		pkgBar = sample.ModulePath + "/bar"
		foobar = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
			"bar/bar.go": "// Package bar\npackage bar\n\nconst Bar = 21",
		}
	)

	// First fetch and insert a version containing package foo, and verify that
	// foo can be retrieved.
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)
	if _, err := FetchAndUpdateState(ctx, sample.ModulePath, version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", sample.ModulePath, version, proxyClient, sourceClient, testDB, err)
	}

	if _, err := testDB.LegacyGetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); err != nil {
		t.Error(err)
	}

	// Now re-fetch and verify that contents were overwritten.
	proxyClient, teardownProxy = proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    version,
			Files:      foobar,
		},
	})
	defer teardownProxy()

	if _, err := FetchAndUpdateState(ctx, sample.ModulePath, version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
	want := &internal.LegacyVersionedPackage{
		LegacyModuleInfo: internal.LegacyModuleInfo{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        sample.ModulePath,
				Version:           version,
				CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
				VersionType:       "release",
				IsRedistributable: true,
				HasGoMod:          false,
				SourceInfo:        source.NewGitHubInfo("https://"+sample.ModulePath, "", sample.VersionString),
			},
			LegacyReadmeFilePath: "README.md",
			LegacyReadmeContents: "This is a readme",
		},
		LegacyPackage: internal.LegacyPackage{
			Path:              sample.ModulePath + "/bar",
			Name:              "bar",
			Synopsis:          "Package bar",
			DocumentationHTML: html("Bar returns the string &#34;bar&#34;."),
			V1Path:            sample.ModulePath + "/bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"MIT"}, FilePath: "LICENSE"},
			},
			IsRedistributable: true,
			GOOS:              "linux",
			GOARCH:            "amd64",
		},
	}
	got, err := testDB.LegacyGetPackage(ctx, pkgBar, internal.UnknownModulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"), cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Errorf("testDB.LegacyGetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", pkgBar, version, diff)
	}

	// Now re-fetch and verify that contents were overwritten.
	proxyClient, teardownProxy = proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()
	if _, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB, testAppVersion); !errors.Is(err, derrors.DBModuleInsertInvalid) {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
}

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

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
				ModulePath:        "github.com/my/module",
				Version:           sample.VersionString,
				CommitTime:        testProxyCommitTime,
				SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", sample.VersionString),
				VersionType:       "release",
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
						VersionType:       "release",
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
						VersionType:       "release",
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
						VersionType:       "release",
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
						VersionType:       "release",
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
						VersionType:       "release",
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
						VersionType:       "release",
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

func TestFetchAndUpdateState_Timeout(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestClient(t, nil)
	defer teardownProxy()

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, http.StatusInternalServerError)
}

// Check that when the proxy says fetch timed out, we return a 5xx error so
// that we automatically try to fetch it again later.
func TestFetchAndUpdateState_ProxyTimedOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	proxyServer := proxy.NewServer(nil)
	proxyServer.AddRoute(
		fmt.Sprintf("/%s/@v/%s.info", sample.ModulePath, sample.VersionString),
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found: fetch timed out", http.StatusNotFound)
		})
	proxyClient, teardownProxy, err := proxy.NewClientForServer(proxyServer)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownProxy()

	fetchAndCheckStatus(ctx, t, proxyClient, sample.ModulePath, sample.VersionString, derrors.ToStatus(derrors.ProxyTimedOut))
}

func fetchAndCheckStatus(ctx context.Context, t *testing.T, proxyClient *proxy.Client, modulePath, version string, wantCode int) {
	t.Helper()
	sourceClient := source.NewClient(sourceTimeout)
	code, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB, testAppVersion)
	switch code {
	case http.StatusOK:
		if err != nil {
			t.Fatalf("FetchAndUpdateState: %v", err)
		}
	case derrors.ToStatus(derrors.AlternativeModule):
		if !errors.Is(err, derrors.AlternativeModule) {
			t.Fatalf("FetchAndUpdateState: %v; want = %v", err, derrors.AlternativeModule)
		}
	case http.StatusNotFound:
		if !errors.Is(err, derrors.NotFound) {
			t.Fatalf("FetchAndUpdateState: %v; want = %v", err, derrors.NotFound)
		}
	case http.StatusForbidden:
		if !errors.Is(err, derrors.Excluded) {
			t.Fatalf("FetchAndUpdateState: %v; want = %v", err, derrors.NotFound)
		}
	case http.StatusInternalServerError:
		// The only case where we check for a status 500 is in
		// TestFetchAndUpdateState_Timeout.
		wantErrString := "deadline exceeded"
		if !strings.Contains(err.Error(), wantErrString) {
			t.Fatalf("FetchAndUpdateState: %v; want error containing %q", err, wantErrString)
		}
		return
	}
	if code != wantCode {
		t.Fatalf("got %d; want = %d", code, wantCode)
	}

	_, err = testDB.GetModuleInfo(ctx, modulePath, version)
	switch code {
	case http.StatusOK, hasIncompletePackagesCode:
		if err != nil {
			t.Fatalf("testDB.GetModuleInfo: %v", err)
		}
	default:
		if !errors.Is(err, derrors.NotFound) {
			t.Fatalf("got %v, want Is(NotFound)", err)
		}
	}
	if semver.IsValid(version) {
		if _, err := testDB.GetModuleVersionState(ctx, modulePath, version); err != nil {
			t.Fatal(err)
		}
	}
	vm, err := testDB.GetVersionMap(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != wantCode {
		t.Fatalf("testDB.GetVersionMap(ctx, %q, %q): status = %d, want = %d", modulePath, version, vm.Status, wantCode)
	}
}

func checkPackageVersionStates(ctx context.Context, t *testing.T, modulePath, version string, wantStates []*internal.PackageVersionState) {
	t.Helper()
	gotStates, err := testDB.GetPackageVersionStatesForModule(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(gotStates, func(i, j int) bool {
		return gotStates[i].PackagePath < gotStates[j].PackagePath
	})
	if diff := cmp.Diff(wantStates, gotStates); diff != "" {
		t.Errorf("testDB.GetPackageVersionStatesForModule(ctx, %q, %q) mismatch (-want +got):\n%s",
			modulePath, version, diff)
	}
}

func checkPackage(ctx context.Context, t *testing.T, pkgPath string) {
	t.Helper()
	if _, err := testDB.LegacyGetPackage(ctx, pkgPath, internal.UnknownModulePath, sample.VersionString); err != nil {
		t.Fatal(err)
	}
	modulePath, version, isPackage, err := testDB.GetPathInfo(ctx, pkgPath, internal.UnknownModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}
	if !isPackage {
		t.Fatalf("testDB.GetPathInfo(%q, %q, %q): isPackage = false; want = true",
			pkgPath, internal.UnknownModulePath, sample.VersionString)
	}
	dir, err := testDB.GetDirectory(ctx, pkgPath, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if dir.Package == nil || dir.Package.Documentation == nil {
		t.Fatalf("testDB.GetDirectory(%q, %q, %q): documentation should not be nil",
			pkgPath, modulePath, version)
	}
}
