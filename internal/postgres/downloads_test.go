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

func TestUpdateableDownloadDates(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := t.Context()

	d1 := civil.Date{Year: 2025, Month: 11, Day: 1}
	d2 := civil.Date{Year: 2025, Month: 11, Day: 2}
	d3 := civil.Date{Year: 2025, Month: 11, Day: 3}
	d4 := civil.Date{Year: 2025, Month: 11, Day: 4}
	d5 := civil.Date{Year: 2025, Month: 11, Day: 5}
	d6 := civil.Date{Year: 2025, Month: 11, Day: 6}
	d7 := civil.Date{Year: 2025, Month: 11, Day: 7}
	d8 := civil.Date{Year: 2025, Month: 11, Day: 8}
	today := civil.Date{Year: 2025, Month: 11, Day: 9}

	// Reject invalid date.
	if _, err := testDB.UpdateableDownloadDates(ctx, civil.Date{}); !errors.Is(err, derrors.InvalidArgument) {
		t.Errorf("UpdateableDownloadDates(zero): got %v, want InvalidArgument", err)
	}

	// When today <= MinDownloadDate, no dates are eligible.
	got, err := testDB.UpdateableDownloadDates(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("today == MinDownloadDate: got %v, want empty", got)
	}

	// When the table is empty, dates start at MinDownloadDate (2025-11-01),
	// returned most recent first.
	got, err = testDB.UpdateableDownloadDates(ctx, civil.Date{Year: 2025, Month: 11, Day: 3})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]civil.Date{d2, d1}, got); diff != "" {
		t.Errorf("empty table mismatch (-want +got):\n%s", diff)
	}

	// d1: missing (before earliest entry) -> should be returned.
	// d2: succeeded (status = 200) -> should not be returned.
	if err := testDB.UpsertModuleDownloadState(ctx, d2, 200, ""); err != nil {
		t.Fatal(err)
	}
	// d3: missing (gap between entries) -> should be returned.
	// d4: failed recently (< 48h ago) -> should not be returned.
	if err := testDB.UpsertModuleDownloadState(ctx, d4, 500, "recent failure"); err != nil {
		t.Fatal(err)
	}
	// d5: failed (status = 500) > 48h ago -> should be returned.
	if err := testDB.UpsertModuleDownloadState(ctx, d5, 500, "old failure"); err != nil {
		t.Fatal(err)
	}
	// d6: missing export (status = 404) > 48h ago -> should be returned.
	if err := testDB.UpsertModuleDownloadState(ctx, d6, 404, "not found"); err != nil {
		t.Fatal(err)
	}
	const backdate = `
		UPDATE module_download_states
		SET updated_at = CURRENT_TIMESTAMP - INTERVAL '49 hours'
		WHERE date = $1
	`
	for _, d := range []civil.Date{d5, d6} {
		if _, err := testDB.db.Exec(ctx, backdate, d); err != nil {
			t.Fatal(err)
		}
	}
	// d7: succeeded (status = 200) -> should not be returned.
	if err := testDB.UpsertModuleDownloadState(ctx, d7, 200, ""); err != nil {
		t.Fatal(err)
	}
	// d8: missing (after latest entry) -> should be returned.

	got, err = testDB.UpdateableDownloadDates(ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	want := []civil.Date{d8, d6, d5, d3, d1}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("UpdateableDownloadDates mismatch (-want +got):\n%s", diff)
	}
}

func TestLatestDownloadDate(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := t.Context()

	if d, ok, err := testDB.LatestDownloadDate(ctx); err != nil || ok {
		t.Fatalf("empty table: got (%v, %v, %v), want (_, false, nil)", d, ok, err)
	}

	d1 := civil.Date{Year: 2026, Month: 10, Day: 19}
	d2 := civil.Date{Year: 2026, Month: 10, Day: 20}
	d3 := civil.Date{Year: 2026, Month: 10, Day: 21}

	if err := testDB.UpsertModuleDownloadState(ctx, d3, 500, "failed"); err != nil {
		t.Fatal(err)
	}
	if d, ok, err := testDB.LatestDownloadDate(ctx); err != nil || ok {
		t.Fatalf("only failures: got (%v, %v, %v), want (_, false, nil)", d, ok, err)
	}

	if err := testDB.UpsertModuleDownloadState(ctx, d1, 200, ""); err != nil {
		t.Fatal(err)
	}
	if err := testDB.UpsertModuleDownloadState(ctx, d2, 200, ""); err != nil {
		t.Fatal(err)
	}
	got, ok, err := testDB.LatestDownloadDate(ctx)
	if err != nil || !ok || got != d2 {
		t.Errorf("got (%v, %v, %v), want (%v, true, nil)", got, ok, err, d2)
	}
}

func TestUpdateModuleDownloads30d(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := t.Context()

	latest := civil.Date{Year: 2026, Month: 10, Day: 20}
	inWindow := latest.AddDays(-29)
	beforeWindow := latest.AddDays(-30)

	if err := testDB.UpsertModuleDownloads(ctx, []DownloadInfo{
		{ModulePath: "example.com/a", Date: beforeWindow, Downloads: 1000, UniqueIPs: 100},
		{ModulePath: "example.com/a", Date: inWindow, Downloads: 10, UniqueIPs: 1},
		{ModulePath: "example.com/a", Date: latest, Downloads: 20, UniqueIPs: 2},
		{ModulePath: "example.com/b", Date: beforeWindow, Downloads: 50, UniqueIPs: 5},
	}); err != nil {
		t.Fatal(err)
	}
	if err := testDB.UpsertModuleDownloadState(ctx, latest, 200, ""); err != nil {
		t.Fatal(err)
	}
	if err := testDB.UpdateModuleDownloads30d(ctx); err != nil {
		t.Fatal(err)
	}

	const q = `
		SELECT p.path, v.downloads_30d
		FROM module_downloads_30d v
		JOIN paths p ON p.id = v.module_path_id
	`
	got := map[string]int64{}
	err := testDB.db.RunQuery(ctx, q, func(rows *sql.Rows) error {
		var p string
		var n int64
		if err := rows.Scan(&p, &n); err != nil {
			return err
		}
		got[p] = n
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"example.com/a": 30}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("module_downloads_30d mismatch (-want +got):\n%s", diff)
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
