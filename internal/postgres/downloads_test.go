// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"errors"
	"testing"

	"cloud.google.com/go/civil"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/derrors"
)

func TestUpsertModuleDownloadState(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := t.Context()

	date := civil.Date{Year: 2026, Month: 10, Day: 20}

	// Insert initial error state.
	if err := testDB.UpsertModuleDownloadState(ctx, date, 500, "initial failure"); err != nil {
		t.Fatal(err)
	}

	type stateRow struct {
		Date   civil.Date
		Status int
		Error  string
		Tries  int
	}
	readState := func(d civil.Date) stateRow {
		t.Helper()
		var r stateRow
		const q = `SELECT date, status, error, tries FROM module_download_states WHERE date = $1`
		if err := testDB.db.QueryRow(ctx, q, d).Scan(&r.Date, &r.Status, &r.Error, &r.Tries); err != nil {
			t.Fatal(err)
		}
		return r
	}

	got := readState(date)
	want := stateRow{Date: date, Status: 500, Error: "initial failure", Tries: 1}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch after insert (-want +got):\n%s", diff)
	}

	// Update existing state to 200 OK.
	if err := testDB.UpsertModuleDownloadState(ctx, date, 200, ""); err != nil {
		t.Fatal(err)
	}
	got = readState(date)
	want = stateRow{Date: date, Status: 200, Error: "", Tries: 2}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch after update (-want +got):\n%s", diff)
	}

	// Reject invalid dates.
	for _, bad := range []civil.Date{{}, {Year: 2026, Month: 2, Day: 30}} {
		if err := testDB.UpsertModuleDownloadState(ctx, bad, 200, ""); !errors.Is(err, derrors.InvalidArgument) {
			t.Errorf("UpsertModuleDownloadState(%v): got %v, want InvalidArgument", bad, err)
		}
	}
}

func TestUpsertModuleDownloads(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := t.Context()

	d1 := civil.Date{Year: 2026, Month: 10, Day: 20}
	d2 := civil.Date{Year: 2026, Month: 10, Day: 21}

	initial := []DownloadInfo{
		{ModulePath: "example.com/a", Date: d1, Downloads: 100, UniqueIPs: 10},
		{ModulePath: "example.com/b", Date: d1, Downloads: 50, UniqueIPs: 5},
		{ModulePath: "example.com/a", Date: d2, Downloads: 200, UniqueIPs: 20},
	}
	if err := testDB.UpsertModuleDownloads(ctx, initial); err != nil {
		t.Fatal(err)
	}

	// Upsert again to update example.com/a on d1 and insert example.com/c on d2.
	update := []DownloadInfo{
		{ModulePath: "example.com/a", Date: d1, Downloads: 150, UniqueIPs: 15},
		{ModulePath: "example.com/c", Date: d2, Downloads: 75, UniqueIPs: 8},
	}
	if err := testDB.UpsertModuleDownloads(ctx, update); err != nil {
		t.Fatal(err)
	}

	const q = `
		SELECT p.path, md.date, md.downloads, md.unique_ips
		FROM module_downloads md
		JOIN paths p ON p.id = md.module_path_id
		ORDER BY p.path, md.date
	`
	var got []DownloadInfo
	err := testDB.db.RunQuery(ctx, q, func(rows *sql.Rows) error {
		var info DownloadInfo
		if err := rows.Scan(&info.ModulePath, &info.Date, &info.Downloads, &info.UniqueIPs); err != nil {
			return err
		}
		got = append(got, info)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []DownloadInfo{
		{ModulePath: "example.com/a", Date: d1, Downloads: 150, UniqueIPs: 15},
		{ModulePath: "example.com/a", Date: d2, Downloads: 200, UniqueIPs: 20},
		{ModulePath: "example.com/b", Date: d1, Downloads: 50, UniqueIPs: 5},
		{ModulePath: "example.com/c", Date: d2, Downloads: 75, UniqueIPs: 8},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("UpsertModuleDownloads mismatch (-want +got):\n%s", diff)
	}

	// Reject invalid dates and duplicate (ModulePath, Date) pairs.
	invalid := []DownloadInfo{
		{ModulePath: "example.com/bad", Date: civil.Date{Year: 2026, Month: 2, Day: 30}},
	}
	if err := testDB.UpsertModuleDownloads(ctx, invalid); !errors.Is(err, derrors.InvalidArgument) {
		t.Errorf("UpsertModuleDownloads(invalid date): got %v, want InvalidArgument", err)
	}
	dups := []DownloadInfo{
		{ModulePath: "example.com/dup", Date: d1, Downloads: 1, UniqueIPs: 1},
		{ModulePath: "example.com/dup", Date: d1, Downloads: 2, UniqueIPs: 2},
	}
	if err := testDB.UpsertModuleDownloads(ctx, dups); !errors.Is(err, derrors.InvalidArgument) {
		t.Errorf("UpsertModuleDownloads(duplicate): got %v, want InvalidArgument", err)
	}
}
