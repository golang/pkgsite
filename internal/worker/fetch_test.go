// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/testhelper"
)

var sourceTimeout = 1 * time.Second

var buildConstraintsMod = &proxy.TestModule{
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
}

// Check that when the proxy says it does not have module@version,
// we delete it from the database.
func TestFetchAndUpdateState_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	const (
		modulePath = "github.com/take/down"
		version    = "v1.0.0"
	)

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files: map[string]string{
				"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
				"README.md":  "This is a readme",
				"LICENSE":    testhelper.MITLicense,
			},
		},
	})
	sourceClient := source.NewClient(sourceTimeout)

	checkStatus := func(want int) {
		t.Helper()
		vs, err := testDB.GetModuleVersionState(ctx, modulePath, version)
		if err != nil {
			t.Fatal(err)
		}
		if vs.Status != want {
			t.Fatalf("testDB.GetModuleVersionState(ctx, %q, %q): status = %v, want = %d", modulePath, version, vs.Status, want)
		}
		vm, err := testDB.GetVersionMap(ctx, modulePath, modulePath, version)
		if err != nil {
			t.Fatal(err)
		}
		if vm.Status != want {
			t.Fatalf("testDB.GetVersionMap(ctx, %q, %q, %q): status = %d, want = %d", modulePath, modulePath, version, vm.Status, want)
		}
	}

	// Fetch a module@version that the proxy serves successfully.
	if _, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB); err != nil {
		t.Fatal(err)
	}

	// Verify that the module status is recorded correctly, and that the version is in the DB.
	checkStatus(http.StatusOK)

	if _, err := testDB.GetModuleInfo(ctx, modulePath, version); err != nil {
		t.Fatal(err)
	}

	gotStates, err := testDB.GetPackageVersionStatesForModule(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	wantStates := []*internal.PackageVersionState{
		{
			PackagePath: "github.com/take/down/foo",
			ModulePath:  modulePath,
			Version:     version,
			Status:      http.StatusOK,
		},
	}
	if diff := cmp.Diff(wantStates, gotStates); diff != "" {
		t.Errorf("testDB.GetPackageVersionStatesForModule(ctx, %q, %q) mismatch (-want +got):\n%s", modulePath, version, diff)
	}

	teardownProxy()

	// Take down the module, by having the proxy serve a 404/410 for it.
	proxyMux := proxy.TestProxy([]*proxy.TestModule{}) // serve no versions, not even the defaults.
	proxyMux.HandleFunc(fmt.Sprintf("/%s/@v/%s.info", modulePath, version),
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "taken down", http.StatusGone) })
	proxyClient, teardownProxy2 := proxy.TestProxyServer(t, proxyMux)
	defer teardownProxy2()

	// Now fetch it again.
	if code, _ := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB); code != http.StatusNotFound && code != http.StatusGone {
		t.Fatalf("FetchAndUpdateState(ctx, %q, %q, proxyClient, sourceClient, testDB): got code %d, want 404/410", modulePath, version, code)
	}

	// The new state should have a status of Not Found.
	checkStatus(http.StatusNotFound)

	// The module should no longer be in the database.
	if _, err := testDB.GetModuleInfo(ctx, modulePath, version); !errors.Is(err, derrors.NotFound) {
		t.Fatalf("got %v, want NotFound", err)
	}

	gotStates, err = testDB.GetPackageVersionStatesForModule(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if gotStates != nil {
		t.Errorf("testDB.GetPackageVersionStatesForModule(%q, %q) = %v; want = nil", modulePath, version, gotStates)
	}
}

func TestFetchAndUpdateState_Excluded(t *testing.T) {
	// Check that an excluded module is not processed, and is marked excluded in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, nil)
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	const (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
	)
	if err := testDB.InsertExcludedPrefix(ctx, "github.com/my/m", "user", "for testing"); err != nil {
		t.Fatal(err)
	}

	checkModuleNotFound(t, ctx, modulePath, version, proxyClient, sourceClient, http.StatusForbidden, derrors.Excluded)
}

func checkModuleNotFound(t *testing.T, ctx context.Context, modulePath, version string, proxyClient *proxy.Client, sourceClient *source.Client, wantCode int, wantErr error) {
	code, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	if code != wantCode || !errors.Is(err, wantErr) {
		t.Fatalf("got %d, %v; want %d, Is(err, %v)", code, err, wantCode, wantErr)
	}
	_, err = testDB.GetModuleInfo(ctx, modulePath, version)
	if !errors.Is(err, derrors.NotFound) {
		t.Fatalf("got %v, want Is(NotFound)", err)
	}
	vs, err := testDB.GetModuleVersionState(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vs.Status != wantCode {
		t.Fatalf("testDB.GetModuleVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, vs.Status, wantCode)
	}
	vm, err := testDB.GetVersionMap(ctx, modulePath, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != wantCode {
		t.Fatalf("testDB.GetVersionMap(ctx, %q, %q, %q): status=%v, want %d", modulePath, modulePath, version, vm.Status, wantCode)
	}
}

func TestFetchAndUpdateState_BadRequestedVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{buildConstraintsMod})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	const (
		modulePath = "build.constraints/module"
		version    = "badversion"
		want       = http.StatusNotFound
	)

	code, _ := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	if code != want {
		t.Fatalf("got code %d, want %d", code, want)
	}
	_, err := testDB.GetModuleVersionState(ctx, modulePath, version)
	if !errors.Is(err, derrors.NotFound) {
		t.Fatal(err)
	}
	vm, err := testDB.GetVersionMap(ctx, modulePath, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != http.StatusNotFound {
		t.Fatalf("testDB.GetVersionMap(ctx, %q, %q, %q): status=%v, want %d", modulePath, modulePath, version, vm.Status, http.StatusNotFound)
	}
}

func TestFetchAndUpdateState_Incomplete(t *testing.T) {
	// Check that we store the special "incomplete" status in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{buildConstraintsMod})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	const (
		modulePath = "build.constraints/module"
		version    = "v1.0.0"
		want       = hasIncompletePackagesCode
	)

	code, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	if err != nil {
		t.Fatal(err)
	}
	if code != want {
		t.Fatalf("got code %d, want %d", code, want)
	}
	vs, err := testDB.GetModuleVersionState(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vs.Status != want {
		t.Fatalf("testDB.GetModuleVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, vs.Status, want)
	}
	vm, err := testDB.GetVersionMap(ctx, modulePath, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != want {
		t.Fatalf("testDB.GetVersionMap(ctx, %q, %q, %q): status=%v, want %d", modulePath, modulePath, version, vm.Status, want)
	}
	gotStates, err := testDB.GetPackageVersionStatesForModule(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	wantStates := []*internal.PackageVersionState{
		{
			PackagePath: "build.constraints/module/cpu",
			ModulePath:  "build.constraints/module",
			Version:     "v1.0.0",
			Status:      200,
		},
		{
			PackagePath: "build.constraints/module/ignore",
			ModulePath:  "build.constraints/module",
			Version:     "v1.0.0",
			Status:      600,
		},
	}
	sort.Slice(gotStates, func(i, j int) bool {
		return gotStates[i].PackagePath < gotStates[j].PackagePath
	})
	if diff := cmp.Diff(wantStates, gotStates); diff != "" {
		t.Errorf("testDB.GetPackageVersionStatesForModule(ctx, %q, %q) mismatch (-want +got):\n%s", modulePath, version, diff)
	}
}

func TestFetchAndUpdateState_Mismatch(t *testing.T) {
	// Check that an excluded module is not processed, and is marked excluded in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	const (
		modulePath = "github.com/mis/match"
		version    = "v1.0.0"
		goModPath  = "other"
	)
	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files: map[string]string{
				"go.mod":     "module " + goModPath,
				"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			},
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	code, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	wantErr := derrors.AlternativeModule
	wantCode := derrors.ToHTTPStatus(wantErr)
	if code != wantCode || !errors.Is(err, wantErr) {
		t.Fatalf("got %d, %v; want %d, Is(err, derrors.AlternativeModule)", code, err, wantCode)
	}
	_, err = testDB.GetModuleInfo(ctx, modulePath, version)
	if !errors.Is(err, derrors.NotFound) {
		t.Fatalf("got %v, want Is(NotFound)", err)
	}
	vs, err := testDB.GetModuleVersionState(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}

	if vs.Status != wantCode {
		t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, vs.Status, wantCode)
	}

	if vs.GoModPath != goModPath {
		t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q): goModPath=%q, want %q", modulePath, version, vs.GoModPath, goModPath)
	}

	vm, err := testDB.GetVersionMap(ctx, modulePath, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != wantCode {
		t.Fatalf("testDB.GetVersionMap(ctx, %q, %q, %q): status=%v, want %d", modulePath, modulePath, version, vm.Status, wantCode)
	}
}

func TestFetchAndUpdateState_DeleteOlder(t *testing.T) {
	// Check that fetching an alternative module deletes all older versions of that
	// module from search_documents (but not versions).
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	const (
		modulePath      = "github.com/m"
		mismatchVersion = "v1.0.0"
		olderVersion    = "v0.9.0"
	)

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			// mismatched version; will cause deletion
			ModulePath: modulePath,
			Version:    mismatchVersion,
			Files: map[string]string{
				"go.mod":     "module other",
				"foo/foo.go": "package foo",
			},
		},
		{
			// older version; should be deleted
			ModulePath: modulePath,
			Version:    olderVersion,
			Files: map[string]string{
				"foo/foo.go": "package foo",
			},
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	if _, err := FetchAndUpdateState(ctx, modulePath, olderVersion, proxyClient, sourceClient, testDB); err != nil {
		t.Fatal(err)
	}
	gotModule, gotVersion, gotFound := postgres.GetFromSearchDocuments(ctx, t, testDB, modulePath+"/foo")
	if !gotFound || gotModule != modulePath || gotVersion != olderVersion {
		t.Fatalf("got (%q, %q, %t), want (%q, %q, true)", gotModule, gotVersion, gotFound, modulePath, olderVersion)
	}

	code, _ := FetchAndUpdateState(ctx, modulePath, mismatchVersion, proxyClient, sourceClient, testDB)
	if want := derrors.ToHTTPStatus(derrors.AlternativeModule); code != want {
		t.Fatalf("got %d, want %d", code, want)
	}

	if _, _, gotFound := postgres.GetFromSearchDocuments(ctx, t, testDB, modulePath+"/foo"); gotFound {
		t.Fatal("older version found in search documents")
	}
}

func TestSkipIncompletePackage(t *testing.T) {
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
	var (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
	)
	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      badModule,
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	res, err := fetchAndInsertModule(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	if err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
	if !res.HasIncompletePackages {
		t.Errorf("fetchAndInsertVersion(%q, %q, %v, %v, %v): hasIncompletePackages=false, want true",
			modulePath, version, proxyClient, sourceClient, testDB)
	}

	pkgFoo := modulePath + "/foo"
	if _, err := testDB.GetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); err != nil {
		t.Errorf("got %v, want nil", err)
	}
	pkgBar := modulePath + "/bar"
	if _, err := testDB.GetPackage(ctx, pkgBar, internal.UnknownModulePath, version); !errors.Is(err, derrors.NotFound) {
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
	var (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
	)
	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      trimmedModule,
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	res, err := fetchAndInsertModule(ctx, modulePath, version, proxyClient, sourceClient, testDB)
	if err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
	if res.HasIncompletePackages {
		t.Errorf("fetchAndInsertVersion(%q, %q, %v, %v, %v): hasIncompletePackages=true, want false",
			modulePath, version, proxyClient, sourceClient, testDB)
	}

	pkgFoo := modulePath + "/foo"
	if _, err := testDB.GetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); err != nil {
		t.Errorf("got %v, want nil", err)
	}
	pkgBar := modulePath + "/bar"
	if _, err := testDB.GetPackage(ctx, pkgBar, internal.UnknownModulePath, version); err != nil {
		t.Errorf("got %v, want nil", err)
	}
	pkgBaz := modulePath + "/baz"
	if _, err := testDB.GetPackage(ctx, pkgBaz, internal.UnknownModulePath, version); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestFetch_V1Path(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	proxyClient, tearDown := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: "my.mod/foo",
			Version:    "v1.0.0",
			Files: map[string]string{
				"foo.go":  "package foo\nconst Foo = 41",
				"LICENSE": testhelper.MITLicense,
			},
		},
	})
	defer tearDown()
	sourceClient := source.NewClient(sourceTimeout)
	if _, err := fetchAndInsertModule(ctx, "my.mod/foo", "v1.0.0", proxyClient, sourceClient, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion: %v", err)
	}
	pkg, err := testDB.GetPackage(ctx, "my.mod/foo", internal.UnknownModulePath, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pkg.V1Path, "my.mod/foo"; got != want {
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
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
		pkgFoo     = "github.com/my/module/foo"
		pkgBar     = "github.com/my/module/bar"
		foo        = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
		bar = map[string]string{
			"bar/bar.go": "// Package bar\npackage bar\n\nconst Bar = 21",
			"README.md":  "This is another readme",
			"COPYING":    testhelper.MITLicense,
		}
	)

	// First fetch and insert a version containing package foo, and verify that
	// foo can be retrieved.
	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)
	if _, err := fetchAndInsertModule(ctx, modulePath, version, proxyClient, sourceClient, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}

	if _, err := testDB.GetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); err != nil {
		t.Error(err)
	}

	// Now re-fetch and verify that contents were overwritten.
	proxyClient, teardownProxy = proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      bar,
		},
	})
	defer teardownProxy()

	if _, err := fetchAndInsertModule(ctx, modulePath, version, proxyClient, sourceClient, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
	want := &internal.VersionedPackage{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           version,
			CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
			ReadmeFilePath:    "README.md",
			ReadmeContents:    "This is another readme",
			VersionType:       "release",
			IsRedistributable: true,
			HasGoMod:          false,
			SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
		},
		Package: internal.Package{
			Path:              "github.com/my/module/bar",
			Name:              "bar",
			Synopsis:          "Package bar",
			DocumentationHTML: "Bar returns the string &#34;bar&#34;.",
			V1Path:            "github.com/my/module/bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"MIT"}, FilePath: "COPYING"},
			},
			IsRedistributable: true,
			GOOS:              "linux",
			GOARCH:            "amd64",
		},
	}
	got, err := testDB.GetPackage(ctx, pkgBar, internal.UnknownModulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"), cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Errorf("testDB.GetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", pkgBar, version, diff)
	}

	// For good measure, verify that package foo is now NotFound.
	if _, err := testDB.GetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}
}

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

func TestFetchAndInsertVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const goRepositoryURLPrefix = "https://github.com/golang"

	stdlib.UseTestData = true
	defer func() { stdlib.UseTestData = false }()

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
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

	myModuleV100 := &internal.VersionedPackage{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        "github.com/my/module",
			Version:           "v1.0.0",
			CommitTime:        testProxyCommitTime,
			ReadmeFilePath:    "README.md",
			ReadmeContents:    "README FILE FOR TESTING.",
			SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
			VersionType:       "release",
			IsRedistributable: true,
			HasGoMod:          true,
		},
		Package: internal.Package{
			Path:              "github.com/my/module/bar",
			Name:              "bar",
			Synopsis:          "package bar",
			DocumentationHTML: "Bar returns the string &#34;bar&#34;.",
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
		want        *internal.VersionedPackage
		moreWantDoc []string // Additional substrings we expect to see in DocumentationHTML.
		dontWantDoc []string // Substrings we expect not to see in DocumentationHTML.
	}{
		{
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
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
			version:    "v1.0.0",
			pkg:        "nonredistributable.mod/module/bar/baz",
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "nonredistributable.mod/module",
					Version:           "v1.0.0",
					CommitTime:        testProxyCommitTime,
					ReadmeFilePath:    "README.md",
					ReadmeContents:    "README FILE FOR TESTING.",
					VersionType:       "release",
					SourceInfo:        nil,
					IsRedistributable: true,
					HasGoMod:          true,
				},
				Package: internal.Package{
					Path:              "nonredistributable.mod/module/bar/baz",
					Name:              "baz",
					Synopsis:          "package baz",
					DocumentationHTML: "Baz returns the string &#34;baz&#34;.",
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
			version:    "v1.0.0",
			pkg:        "nonredistributable.mod/module/foo",
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "nonredistributable.mod/module",
					Version:           "v1.0.0",
					CommitTime:        testProxyCommitTime,
					ReadmeFilePath:    "README.md",
					ReadmeContents:    "README FILE FOR TESTING.",
					VersionType:       "release",
					SourceInfo:        nil,
					IsRedistributable: true,
					HasGoMod:          true,
				},
				Package: internal.Package{
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
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "std",
					Version:           "v1.12.5",
					CommitTime:        stdlib.TestCommitTime,
					VersionType:       "release",
					ReadmeFilePath:    "README.md",
					ReadmeContents:    "# The Go Programming Language\n",
					SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
					IsRedistributable: true,
					HasGoMod:          true,
				},
				Package: internal.Package{
					Path:              "context",
					Name:              "context",
					Synopsis:          "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					DocumentationHTML: "This example demonstrates the use of a cancelable context to prevent a\ngoroutine leak.",
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
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "std",
					Version:           "v1.12.5",
					CommitTime:        stdlib.TestCommitTime,
					VersionType:       "release",
					ReadmeFilePath:    "README.md",
					ReadmeContents:    "# The Go Programming Language\n",
					SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
					IsRedistributable: true,
					HasGoMod:          true,
				},
				Package: internal.Package{
					Path:              "builtin",
					Name:              "builtin",
					Synopsis:          "Package builtin provides documentation for Go's predeclared identifiers.",
					DocumentationHTML: "int64 is the set of all signed 64-bit integers.",
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
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "std",
					Version:           "v1.12.5",
					CommitTime:        stdlib.TestCommitTime,
					VersionType:       "release",
					ReadmeFilePath:    "README.md",
					ReadmeContents:    "# The Go Programming Language\n",
					SourceInfo:        source.NewGitHubInfo(goRepositoryURLPrefix+"/go", "src", "go1.12.5"),
					IsRedistributable: true,
					HasGoMod:          true,
				},
				Package: internal.Package{
					Path:              "encoding/json",
					Name:              "json",
					Synopsis:          "Package json implements encoding and decoding of JSON as defined in RFC 7159.",
					DocumentationHTML: "The mapping between JSON and Go values is described\nin the documentation for the Marshal and Unmarshal functions.",
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
				`<li><a href="#example-package-CustomMarshalJSON">Package (CustomMarshalJSON)</a></li>`,
				"Decoder.Decode (Stream)",
				`<li><a href="#example-Decoder.Decode-Stream">Decoder.Decode (Stream)</a></li>`,
			},
			dontWantDoc: []string{
				"Example (customMarshalJSON)",
				"Package (customMarshalJSON)",
				"Decoder.Decode (stream)",
			},
		}, {
			modulePath: "build.constraints/module",
			version:    "v1.0.0",
			pkg:        "build.constraints/module/cpu",
			want: &internal.VersionedPackage{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "build.constraints/module",
					Version:           "v1.0.0",
					CommitTime:        testProxyCommitTime,
					VersionType:       "release",
					SourceInfo:        nil,
					IsRedistributable: true,
					HasGoMod:          false,
				},
				Package: internal.Package{
					Path:              "build.constraints/module/cpu",
					Name:              "cpu",
					Synopsis:          "Package cpu implements processor feature detection used by the Go standard library.",
					DocumentationHTML: "const CacheLinePadSize = 3",
					V1Path:            "build.constraints/module/cpu",
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

			if _, err := fetchAndInsertModule(ctx, test.modulePath, test.version, proxyClient, sourceClient, testDB); err != nil {
				t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v): %v", test.modulePath, test.version, proxyClient, sourceClient, testDB, err)
			}

			gotModuleInfo, err := testDB.GetModuleInfo(ctx, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want.ModuleInfo, *gotModuleInfo, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Fatalf("testDB.GetModuleInfo(ctx, %q, %q) mismatch (-want +got):\n%s", test.modulePath, test.version, diff)
			}

			gotPkg, err := testDB.GetPackage(ctx, test.pkg, internal.UnknownModulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}

			sort.Slice(gotPkg.Licenses, func(i, j int) bool {
				return gotPkg.Licenses[i].FilePath < gotPkg.Licenses[j].FilePath
			})
			if diff := cmp.Diff(test.want, gotPkg, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkg, test.version, diff)
			}
			if got, want := gotPkg.DocumentationHTML, test.want.DocumentationHTML; len(want) == 0 && len(got) != 0 {
				t.Errorf("got non-empty documentation but want empty:\ngot: %q\nwant: %q", got, want)
			} else if !strings.Contains(got, want) {
				t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
			}
			for _, want := range test.moreWantDoc {
				if got := gotPkg.DocumentationHTML; !strings.Contains(got, want) {
					t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
				}
			}
			for _, dontWant := range test.dontWantDoc {
				if got := gotPkg.DocumentationHTML; strings.Contains(got, dontWant) {
					t.Errorf("got documentation contains unwanted documentation substring:\ngot: %q\ndontWant (substring): %q", got, dontWant)
				}
			}
		})
	}
}

func TestFetchAndInsertVersionTimeout(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	defer func(oldTimeout time.Duration) {
		fetchTimeout = oldTimeout
	}(fetchTimeout)
	fetchTimeout = 0

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, nil)
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)

	name := "my.mod/version"
	version := "v1.0.0"
	wantErrString := "deadline exceeded"
	_, err := fetchAndInsertModule(context.Background(), name, version, proxyClient, sourceClient, testDB)
	if err == nil || !strings.Contains(err.Error(), wantErrString) {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v, %v) returned error %v, want error containing %q",
			name, version, proxyClient, sourceClient, testDB, err, wantErrString)
	}
}
