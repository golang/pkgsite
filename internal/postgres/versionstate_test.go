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
		statusCode = 500
		fetchErr   = errors.New("bad request")
		goModPath  = "goModPath"
	)
	if err := testDB.UpsertModuleVersionState(ctx, fooVersion.Path, fooVersion.Version, "", fooVersion.Timestamp, statusCode, goModPath, fetchErr); err != nil {
		t.Fatal(err)
	}
	errString := fetchErr.Error()
	wantFooState := &internal.ModuleVersionState{
		ModulePath:         "foo.com/bar",
		Version:            "v1.0.0",
		IndexTimestamp:     now,
		TryCount:           1,
		GoModPath:          &goModPath,
		Error:              &errString,
		Status:             &statusCode,
		NextProcessedAfter: gotVersions[1].CreatedAt.Add(1 * time.Minute),
	}
	gotFooState, err := testDB.GetModuleVersionState(ctx, wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantFooState, gotFooState, ignore); diff != "" {
		t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
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
		if err := testDB.UpsertModuleVersionState(ctx, v.Path, v.Version, "", v.Timestamp, http.StatusOK, goModPath, nil); err != nil {
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
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10) mismatch (-want +got):\n%s", diff)
	}
}
