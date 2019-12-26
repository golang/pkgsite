// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/testing/testhelper"
)

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

	client, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, modulePath, version, map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}),
	})

	checkStatus := func(want int) {
		t.Helper()
		vs, err := testDB.GetVersionState(ctx, modulePath, version)
		if err != nil {
			t.Fatal(err)
		}
		if vs.Status == nil || *vs.Status != want {
			t.Fatalf("testDB.GetVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, vs.Status, want)
		}
	}

	// Fetch a module@version that the proxy serves successfully.
	if _, err := fetchAndUpdateState(ctx, modulePath, version, client, testDB); err != nil {
		t.Fatal(err)
	}

	// Verify that the module status is recorded correctly, and that the version is in the DB.
	checkStatus(http.StatusOK)

	if _, err := testDB.GetVersionInfo(ctx, modulePath, version); err != nil {
		t.Fatal(err)
	}

	teardownProxy()

	// Take down the module, by having the proxy serve a 404/410 for it.
	proxyMux := proxy.TestProxy([]*proxy.TestVersion{}) // serve no versions, not even the defaults.
	proxyMux.HandleFunc(fmt.Sprintf("/%s/@v/%s.info", modulePath, version),
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "taken down", http.StatusGone) })
	client, teardownProxy2 := proxy.TestProxyServer(t, proxyMux)
	defer teardownProxy2()

	// Now fetch it again.
	if code, _ := fetchAndUpdateState(ctx, modulePath, version, client, testDB); code != http.StatusNotFound && code != http.StatusGone {
		t.Fatalf("fetchAndUpdateState(ctx, %q, %q, client, testDB): got code %d, want 404/410", modulePath, version, code)
	}

	// The new state should have a status of Not Found.
	checkStatus(http.StatusNotFound)

	// The module should no longer be in the database.
	if _, err := testDB.GetVersionInfo(ctx, modulePath, version); !errors.Is(err, derrors.NotFound) {
		t.Fatalf("got %v, want NotFound", err)
	}
}

func TestFetchAndUpdateState_Excluded(t *testing.T) {
	// Check that an excluded module is not processed, and is marked excluded in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	client, teardownProxy := proxy.SetupTestProxy(t, nil)
	defer teardownProxy()

	const (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
	)
	if err := testDB.InsertExcludedPrefix(ctx, "github.com/my/m", "user", "for testing"); err != nil {
		t.Fatal(err)
	}

	code, err := fetchAndUpdateState(ctx, modulePath, version, client, testDB)
	wantCode := http.StatusForbidden
	if code != wantCode || !errors.Is(err, derrors.Excluded) {
		t.Fatalf("got %d, %v; want %d, Is(err, derrors.Excluded)", code, err, wantCode)
	}
	_, err = testDB.GetVersionInfo(ctx, modulePath, version)
	if !errors.Is(err, derrors.NotFound) {
		t.Fatalf("got %v, want Is(NotFound)", err)
	}
	vs, err := testDB.GetVersionState(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	var gotStatus int
	if vs.Status != nil {
		gotStatus = *vs.Status
	}
	if gotStatus != wantCode {
		t.Fatalf("testDB.GetVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, gotStatus, wantCode)
	}
}

func TestFetchAndUpdateState_Incomplete(t *testing.T) {
	// Check that we store the special "incomplete" status in module_version_states.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	client, teardownProxy := proxy.SetupTestProxy(t, nil)
	defer teardownProxy()

	const (
		modulePath = "build.constraints/module"
		version    = "v1.0.0"
		want       = fetch.HasIncompletePackagesCode
	)

	code, err := fetchAndUpdateState(ctx, modulePath, version, client, testDB)
	if err != nil {
		t.Fatal(err)
	}
	if code != want {
		t.Fatalf("got code %d, want %d", code, want)
	}
	vs, err := testDB.GetVersionState(ctx, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if vs.Status == nil || *vs.Status != want {
		t.Fatalf("testDB.GetVersionState(ctx, %q, %q): status=%v, want %d", modulePath, version, vs.Status, want)
	}
}
