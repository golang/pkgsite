// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
)

func TestEncodeDuration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []time.Duration{
		1 * time.Second,
		1 * time.Hour,
		168 * time.Hour,  // one week
		8760 * time.Hour, // one year
	}

	now := NowTruncated()
	// This query executes some timestamp arithmetic in postgres, so that we can
	// verify the encoded duration is behaving as expected. In the absence of a
	// schema, the type specifications (::INTERVAL) are necessary for postgres to
	// correctly interpret the encoded values.
	query := `SELECT $1::TIMESTAMPTZ + $2::INTERVAL;`
	for _, test := range tests {
		t.Run(fmt.Sprint(test), func(t *testing.T) {
			row := testDB.QueryRowContext(ctx, query, now, encodeDuration(test))
			var later time.Time
			if err := row.Scan(&later); err != nil {
				t.Fatalf("row.Scan(): %v", err)
			}
			if got := later.Sub(now); got != test {
				t.Errorf("got %v later, want %v", got, test)
			}
		})
	}
}

func TestVersionState(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// verify that latest index timestamp works
	initialTime, err := testDB.LatestIndexTimestamp(ctx)
	if err != nil {
		t.Fatalf("testDB.LatestIndexTimestamp(ctx): %v", err)
	}
	if want := (time.Time{}); initialTime != want {
		t.Errorf("testDB.LatestIndexTimestamp(ctx) = %v, want %v", initialTime, want)
	}

	now := NowTruncated().UTC()
	fooVersion := &internal.IndexVersion{Path: "foo.com/bar", Version: "v1.0.0", Timestamp: now}
	bazVersion := &internal.IndexVersion{Path: "baz.com/quux", Version: "v2.0.1", Timestamp: now.Add(10 * time.Second)}
	versions := []*internal.IndexVersion{fooVersion, bazVersion}
	if err := testDB.InsertIndexVersions(ctx, versions); err != nil {
		t.Fatalf("testDB.InsertIndexVersions(ctx, %v): %v", versions, err)
	}

	gotVersions, err := testDB.GetNextVersionsToFetch(ctx, 10)
	if err != nil {
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10): %v", err)
	}

	wantVersions := []*internal.VersionState{
		{ModulePath: "baz.com/quux", Version: "v2.0.1", IndexTimestamp: now.Add(10 * time.Second)},
		{ModulePath: "foo.com/bar", Version: "v1.0.0", IndexTimestamp: now},
	}
	ignore := cmpopts.IgnoreFields(internal.VersionState{}, "CreatedAt", "LastProcessedAt", "NextProcessedAfter")
	if diff := cmp.Diff(wantVersions, gotVersions, ignore); diff != "" {
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10) mismatch (-want +got):\n%s", diff)
	}

	var (
		statusCode = 500
		errorMsg   = "bad request"
		backOff    = 2 * time.Minute
	)
	if err := testDB.UpdateVersionState(ctx, fooVersion.Path, fooVersion.Version, statusCode, errorMsg, backOff); err != nil {
		t.Fatalf("testDB.SetVersionState(ctx, %q, %q, %d, %q): %v", fooVersion.Path,
			versions[0].Version, statusCode, errorMsg, err)
	}

	wantFooState := &internal.VersionState{
		ModulePath:         "foo.com/bar",
		Version:            "v1.0.0",
		IndexTimestamp:     now,
		TryCount:           1,
		Error:              &errorMsg,
		Status:             &statusCode,
		NextProcessedAfter: gotVersions[1].CreatedAt.Add(backOff),
	}
	gotFooState, err := testDB.GetVersionState(ctx, wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatalf("testDB.GetVersionState(ctx, %q, %q): %v", wantFooState.ModulePath, wantFooState.Version, err)
	}
	if diff := cmp.Diff(wantFooState, gotFooState, ignore); diff != "" {
		t.Errorf("testDB.GetVersionState(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}
}
