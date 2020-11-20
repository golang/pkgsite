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

	"github.com/google/go-cmp/cmp"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

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
	if _, err := testDB.GetUnitMeta(ctx, sample.ModulePath+"/bar", internal.UnknownModulePath, sample.VersionString); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
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

// Test that large string literals and slices are trimmed when
// rendering documentation, rather than being included verbatim.
//
// This makes it viable for us to show documentation for packages that
// would otherwise exceed HTML size limit and not get shown at all.
func TestTrimLargeCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
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
		for b.Len() <= godoc.MaxDocumentationHTML {
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
		for b.Len() <= godoc.MaxDocumentationHTML {
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
	if _, err := testDB.GetUnitMeta(ctx, pkgPath, internal.UnknownModulePath, sample.VersionString); err != nil {
		t.Fatal(err)
	}
	um, err := testDB.GetUnitMeta(ctx, pkgPath, internal.UnknownModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}
	if !um.IsPackage() {
		t.Fatalf("testDB.GetUnitMeta(%q, %q, %q): isPackage = false; want = true",
			pkgPath, internal.UnknownModulePath, sample.VersionString)
	}
	dir, err := testDB.GetUnit(ctx, um, internal.WithMain)
	if err != nil {
		t.Fatal(err)
	}
	if dir.Documentation == nil {
		t.Fatalf("testDB.GetUnit(%q, %q, %q): documentation should not be nil",
			um.Path, um.ModulePath, um.Version)
	}
}
