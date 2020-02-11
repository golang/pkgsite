// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/testing/sample"
)

func TestModuleVersionState(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// verify that latest index timestamp works
	initialTime, err := testDB.LatestIndexTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := (time.Time{}); initialTime != want {
		t.Errorf("testDB.LatestIndexTimestamp(ctx) = %v, want %v", initialTime, want)
	}

	now := sample.NowTruncated()
	latest := now.Add(10 * time.Second)
	// insert a FooVersion with no Timestamp, to ensure that it is later updated
	// on conflict.
	initialFooVersion := &internal.IndexVersion{
		Path:    "foo.com/bar",
		Version: "v1.0.0",
	}
	if err := testDB.InsertIndexVersions(ctx, []*internal.IndexVersion{initialFooVersion}); err != nil {
		t.Fatal(err)
	}
	fooVersion := &internal.IndexVersion{
		Path:      "foo.com/bar",
		Version:   "v1.0.0",
		Timestamp: now,
	}
	bazVersion := &internal.IndexVersion{
		Path:      "baz.com/quux",
		Version:   "v2.0.1",
		Timestamp: latest,
	}
	versions := []*internal.IndexVersion{fooVersion, bazVersion}
	if err := testDB.InsertIndexVersions(ctx, versions); err != nil {
		t.Fatal(err)
	}

	gotVersions, err := testDB.GetNextVersionsToFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	wantVersions := []*internal.ModuleVersionState{
		{ModulePath: "baz.com/quux", Version: "v2.0.1", IndexTimestamp: bazVersion.Timestamp},
		{ModulePath: "foo.com/bar", Version: "v1.0.0", IndexTimestamp: fooVersion.Timestamp},
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleVersionState{}, "CreatedAt", "LastProcessedAt", "NextProcessedAfter")
	if diff := cmp.Diff(wantVersions, gotVersions, ignore); diff != "" {
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10) mismatch (-want +got):\n%s", diff)
	}

	var (
		statusCode      = 500
		fetchErr        = errors.New("bad request")
		goModPath       = "goModPath"
		pkgVersionState = &internal.PackageVersionState{
			ModulePath:  "foo.com/bar",
			PackagePath: "foo.com/bar/foo",
			Version:     "v1.0.0",
			Status:      500,
		}
	)
	if err := testDB.UpsertModuleVersionState(ctx, fooVersion.Path, fooVersion.Version, "", fooVersion.Timestamp, statusCode, goModPath, fetchErr, []*internal.PackageVersionState{pkgVersionState}); err != nil {
		t.Fatal(err)
	}
	errString := fetchErr.Error()
	wantFooState := &internal.ModuleVersionState{
		ModulePath:     "foo.com/bar",
		Version:        "v1.0.0",
		IndexTimestamp: now,
		TryCount:       1,
		GoModPath:      &goModPath,
		Error:          &errString,
		Status:         &statusCode,
	}
	gotFooState, err := testDB.GetModuleVersionState(ctx, wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantFooState, gotFooState, ignore); diff != "" {
		t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	gotPVS, err := testDB.GetPackageVersionState(ctx, pkgVersionState.PackagePath, pkgVersionState.ModulePath, pkgVersionState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(pkgVersionState, gotPVS); diff != "" {
		t.Errorf("testDB.GetPackageVersionStates(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	gotPkgVersionStates, err := testDB.GetPackageVersionStatesForModule(ctx,
		wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]*internal.PackageVersionState{pkgVersionState}, gotPkgVersionStates); diff != "" {
		t.Errorf("testDB.GetPackageVersionStates(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	stats, err := testDB.GetVersionStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantStats := &VersionStats{
		LatestTimestamp: latest,
		VersionCounts: map[int]int{
			0:   1,
			500: 1,
		},
	}
	if diff := cmp.Diff(wantStats, stats); diff != "" {
		t.Errorf("testDB.GetVersionStats(ctx) mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateModuleVersionStatesForReprocessing(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	now := sample.NowTruncated()
	goModPath := "goModPath"
	for _, v := range []*internal.IndexVersion{
		{
			Path:      "foo.com/bar",
			Version:   "v1.0.0",
			Timestamp: now,
		},
		{
			Path:      "baz.com/quux",
			Version:   "v2.0.1",
			Timestamp: now,
		},
	} {
		if err := testDB.UpsertModuleVersionState(ctx, v.Path, v.Version, "", v.Timestamp, http.StatusOK, goModPath, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	gotVersions, err := testDB.GetNextVersionsToFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotVersions) != 0 {
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10) = %v; wanted 0 versions", gotVersions)
	}
	if err := testDB.UpdateModuleVersionStatesForReprocessing(ctx, "20190709t112655"); err != nil {
		t.Fatal(err)
	}

	gotVersions, err = testDB.GetNextVersionsToFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	code := http.StatusHTTPVersionNotSupported
	wantVersions := []*internal.ModuleVersionState{
		{ModulePath: "baz.com/quux", Version: "v2.0.1", IndexTimestamp: now, GoModPath: &goModPath, Status: &code},
		{ModulePath: "foo.com/bar", Version: "v1.0.0", IndexTimestamp: now, GoModPath: &goModPath, Status: &code},
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleVersionState{}, "CreatedAt", "LastProcessedAt", "NextProcessedAfter")
	if diff := cmp.Diff(wantVersions, gotVersions, ignore); diff != "" {
		t.Fatalf("testDB.GetNextVersionsToFetch(ctx, 10) mismatch (-want +got):\n%s", diff)
	}
}

func TestGetNextVersionsToFetch(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	now := time.Now()
	mods := []*internal.IndexVersion{
		{"foo.com", "v2.0.1", now},            // latest release version
		{"foo.com/kubernetes", "v2.0.0", now}, // latest release version, lower than above
		{"bar.com", "v3.1.2-alpha", now},      // latest pre-release version
		{"foo.com", "v1.9.3", now},            // next release version
		{"bar.com", "v1.2.3-beta", now},       // next pre-release version
		{"foo.com/kubernetes", "v1.9.0", now}, // non-latest kubernetes
	}

	if err := testDB.InsertIndexVersions(ctx, mods); err != nil {
		t.Fatal(err)
	}
	// Insert a module that we don't expect to retrieve.
	if err := testDB.UpsertModuleVersionState(ctx, "ok.com", "v1.0.0", "", now, 200, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetNextVersionsToFetch(ctx, len(mods)+1)
	if err != nil {
		t.Fatal(err)
	}
	var want []*internal.ModuleVersionState
	for _, iv := range mods {
		want = append(want, &internal.ModuleVersionState{
			ModulePath: iv.Path,
			Version:    iv.Version,
		})
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleVersionState{}, "IndexTimestamp", "CreatedAt", "NextProcessedAfter")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
