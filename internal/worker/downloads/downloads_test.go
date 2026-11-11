// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package downloads

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"cloud.google.com/go/civil"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/postgres"
)

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_downloads_test", m, &testDB)
}

func TestRecords(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 10, Day: 20}
	got, err := collectRecords(t, os.DirFS("testdata"), date)
	if err != nil {
		t.Fatal(err)
	}
	want := []inputRecord{
		{modulePath: "example.com/b", zipCount: 50, zipUniqueIPs: 5},
		{modulePath: "example.com/a", zipCount: 300, zipUniqueIPs: 30},
		{modulePath: "example.com/c", zipCount: 75, zipUniqueIPs: 8},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(inputRecord{})); diff != "" {
		t.Errorf("records mismatch (-want +got):\n%s", diff)
	}
}

func TestReadDownloadInfo(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 10, Day: 20}
	got, err := readDownloadInfo(t.Context(), os.DirFS("testdata"), date)
	if err != nil {
		t.Fatal(err)
	}
	want := []postgres.DownloadInfo{
		{ModulePath: "example.com/a", Date: date, Downloads: 300, UniqueIPs: 30},
		{ModulePath: "example.com/b", Date: date, Downloads: 50, UniqueIPs: 5},
		{ModulePath: "example.com/c", Date: date, Downloads: 75, UniqueIPs: 8},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("readDownloadInfo mismatch (-want +got):\n%s", diff)
	}
}

func TestRecordsErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		date    civil.Date
		wantErr string
	}{
		{
			name:    "missing shard",
			date:    civil.Date{Year: 2026, Month: 10, Day: 22},
			wantErr: "got 1 shards, want 2",
		},
		{
			name:    "bad header",
			date:    civil.Date{Year: 2026, Month: 10, Day: 23},
			wantErr: "got header [wrong header columns]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collectRecords(t, os.DirFS("testdata"), tc.date)
			if err == nil {
				t.Fatalf("got nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got error %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

const validHeader = "module,version,mod_count,zip_count,mod_unique_ips,zip_unique_ips\n"

func TestReadDownloadInfoEdgeCases(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 10, Day: 20}
	dir := "date=2026-10-20/"

	t.Run("header only shard", func(t *testing.T) {
		fsys := fstest.MapFS{
			dir + "counts-00000-of-00001.csv": {Data: []byte(validHeader)},
		}
		got, err := readDownloadInfo(t.Context(), fsys, date)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty slice", got)
		}
	})

	t.Run("non-csv files ignored", func(t *testing.T) {
		fsys := fstest.MapFS{
			dir + "_SUCCESS":                  {Data: nil},
			dir + "notes.txt":                 {Data: []byte("ignore me")},
			dir + "subdir":                    {Mode: fs.ModeDir},
			dir + "counts-00000-of-00001.csv": {Data: []byte(validHeader + "example.com/a,,1,10,1,2\n")},
		}
		got, err := readDownloadInfo(t.Context(), fsys, date)
		if err != nil {
			t.Fatal(err)
		}
		want := []postgres.DownloadInfo{{ModulePath: "example.com/a", Date: date, Downloads: 10, UniqueIPs: 2}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("readDownloadInfo mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("only non-empty versions skipped", func(t *testing.T) {
		fsys := fstest.MapFS{
			dir + "counts-00000-of-00001.csv": {
				Data: []byte(validHeader +
					"example.com/a,v1.0.0,1,10,1,2\n" +
					"example.com/b, ,1,20,1,3\n"),
			},
		}
		got, err := readDownloadInfo(t.Context(), fsys, date)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty slice", got)
		}
	})

	t.Run("uses empty-version row without summing versioned rows", func(t *testing.T) {
		fsys := fstest.MapFS{
			dir + "counts-00000-of-00002.csv": {
				Data: []byte(validHeader +
					"example.com/z,,1,42,1,7\n" +
					"example.com/a,v1.0.0,20,bad,5,bad\n" +
					"example.com/a,v1.1.0,30,200,8,20\n"),
			},
			dir + "counts-00001-of-00002.csv": {
				Data: []byte(validHeader +
					"example.com/a,,50,300,11,25\n"),
			},
		}
		got, err := readDownloadInfo(t.Context(), fsys, date)
		if err != nil {
			t.Fatal(err)
		}
		want := []postgres.DownloadInfo{
			{ModulePath: "example.com/a", Date: date, Downloads: 300, UniqueIPs: 25},
			{ModulePath: "example.com/z", Date: date, Downloads: 42, UniqueIPs: 7},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("readDownloadInfo mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		fsys := fstest.MapFS{
			dir + "counts-00000-of-00001.csv": {Data: []byte(validHeader)},
		}
		if _, err := readDownloadInfo(ctx, fsys, date); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled", err)
		}
	})
}

func TestReadRecordsMapFSErrors(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 10, Day: 20}
	dir := "date=2026-10-20/"

	for _, tc := range []struct {
		name    string
		fsys    fs.FS
		wantErr string
	}{
		{
			name:    "missing directory",
			fsys:    fstest.MapFS{},
			wantErr: "date=2026-10-20",
		},
		{
			name: "empty directory",
			fsys: fstest.MapFS{
				"date=2026-10-20": {Mode: fs.ModeDir},
			},
			wantErr: "no CSV shards",
		},
		{
			name: "unexpected csv filename",
			fsys: fstest.MapFS{
				dir + "other.csv": {Data: []byte(validHeader)},
			},
			wantErr: `unexpected shard filename "other.csv"`,
		},
		{
			name: "extra csv file",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {Data: []byte(validHeader)},
				dir + "extra.csv":                 {Data: []byte(validHeader)},
			},
			wantErr: "got 2 shards, want 1",
		},
		{
			name: "mismatched shard sequence",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00002.csv": {Data: []byte(validHeader)},
				dir + "counts-00002-of-00002.csv": {Data: []byte(validHeader)},
			},
			wantErr: `want "counts-00001-of-00002.csv"`,
		},
		{
			name: "empty shard file",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {Data: nil},
			},
			wantErr: "reading header: EOF",
		},
		{
			name: "wrong header right width",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte("module,version,mod_count,zip_count,mod_unique_ips,wrong\n"),
				},
			},
			wantErr: "got header [module version mod_count zip_count mod_unique_ips wrong]",
		},
		{
			name: "wrong field count in data row",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + "example.com/a,v1.0.0,1,2\n"),
				},
			},
			wantErr: "wrong number of fields",
		},
		{
			name: "empty module path",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + ",,1,2,3,4\n"),
				},
			},
			wantErr: ":2: empty module path",
		},
		{
			name: "non-integer zip count",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + "example.com/a,,1,abc,1,2\n"),
				},
			},
			wantErr: `:2: invalid zip_count "abc"`,
		},
		{
			name: "negative zip count",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + "example.com/a,,1,-5,1,2\n"),
				},
			},
			wantErr: ":2: negative zip_count -5",
		},
		{
			name: "negative unique ips",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + "example.com/a,,1,5,1,-1\n"),
				},
			},
			wantErr: ":2: negative zip_unique_ips -1",
		},
		{
			name: "physical line number after multiline quoted field",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader + "example.com/a,\"v1\n.0.0\",1,5,1,2\nexample.com/b,,1,bad,1,2\n"),
				},
			},
			wantErr: ":4: invalid zip_count",
		},
		{
			name: "duplicate module path in same shard",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00001.csv": {
					Data: []byte(validHeader +
						"example.com/a,,1,10,1,2\n" +
						"example.com/b,,1,20,1,3\n" +
						"example.com/a,,1,30,1,4\n"),
				},
			},
			wantErr: `2026-10-20: duplicate module path "example.com/a"`,
		},
		{
			name: "duplicate module path across shards",
			fsys: fstest.MapFS{
				dir + "counts-00000-of-00002.csv": {
					Data: []byte(validHeader + "example.com/a,,1,10,1,2\n"),
				},
				dir + "counts-00001-of-00002.csv": {
					Data: []byte(validHeader + "example.com/a,,1,30,1,4\n"),
				},
			},
			wantErr: `2026-10-20: duplicate module path "example.com/a"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readDownloadInfo(t.Context(), tc.fsys, date)
			if err == nil {
				t.Fatalf("got nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got error %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestUpdateDownloadInfoForDate(t *testing.T) {
	ctx := t.Context()
	defer postgres.ResetTestDB(testDB, t)
	fsys := os.DirFS("testdata")

	// Successful run for 2026-10-20.
	dOk := civil.Date{Year: 2026, Month: 10, Day: 20}
	if err := updateDownloadInfoForDate(ctx, testDB, fsys, dOk); err != nil {
		t.Fatal(err)
	}

	gotInfos := readDBDownloads(t)
	wantInfos := []postgres.DownloadInfo{
		{ModulePath: "example.com/a", Date: dOk, Downloads: 300, UniqueIPs: 30},
		{ModulePath: "example.com/b", Date: dOk, Downloads: 50, UniqueIPs: 5},
		{ModulePath: "example.com/c", Date: dOk, Downloads: 75, UniqueIPs: 8},
	}
	if diff := cmp.Diff(wantInfos, gotInfos); diff != "" {
		t.Errorf("module_downloads mismatch (-want +got):\n%s", diff)
	}

	status, errMsg := readDBState(t, dOk)
	if status != http.StatusOK || errMsg != "" {
		t.Errorf("state(%s) = (%d, %q), want (%d, \"\")", dOk, status, errMsg, http.StatusOK)
	}

	// Failing run for 2026-10-22 (missing shard -> 500).
	dFail := civil.Date{Year: 2026, Month: 10, Day: 22}
	if err := updateDownloadInfoForDate(ctx, testDB, fsys, dFail); err == nil {
		t.Fatal("got nil, want error for missing shard")
	}
	status, errMsg = readDBState(t, dFail)
	if status != http.StatusInternalServerError || errMsg == "" {
		t.Errorf("state(%s) = (%d, %q), want (%d, non-empty)", dFail, status, errMsg, http.StatusInternalServerError)
	}

	// Missing date directory (fs.ErrNotExist -> 404).
	dMissing := civil.Date{Year: 2026, Month: 10, Day: 25}
	if err := updateDownloadInfoForDate(ctx, testDB, fsys, dMissing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing date: got %v, want fs.ErrNotExist", err)
	}
	status, errMsg = readDBState(t, dMissing)
	if status != http.StatusNotFound || errMsg == "" {
		t.Errorf("state(%s) = (%d, %q), want (%d, non-empty)", dMissing, status, errMsg, http.StatusNotFound)
	}

	// Header-only shard ("no download records" -> 404).
	dEmpty := civil.Date{Year: 2026, Month: 10, Day: 26}
	emptyFS := fstest.MapFS{
		"date=2026-10-26/counts-00000-of-00001.csv": {Data: []byte(validHeader)},
	}
	if err := updateDownloadInfoForDate(ctx, testDB, emptyFS, dEmpty); err == nil {
		t.Fatal("got nil, want error for header-only shard")
	}
	status, errMsg = readDBState(t, dEmpty)
	if status != http.StatusNotFound || errMsg == "" {
		t.Errorf("state(%s) = (%d, %q), want (%d, non-empty)", dEmpty, status, errMsg, http.StatusNotFound)
	}

	// Canceled context must not record a state row.
	dCancel := civil.Date{Year: 2026, Month: 10, Day: 27}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := updateDownloadInfoForDate(cctx, testDB, fsys, dCancel); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: got %v, want context.Canceled", err)
	}
	var count int
	if err := testDB.Underlying().QueryRow(ctx, `SELECT count(*) FROM module_download_states WHERE date = $1`, dCancel).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("canceled date %s wrote %d state rows, want 0", dCancel, count)
	}
}

func TestUpdateDownloadInfo(t *testing.T) {
	ctx := t.Context()
	defer postgres.ResetTestDB(testDB, t)
	fsys := os.DirFS("testdata")

	// Seed state table with successful runs from MinDownloadDate through 2026-10-19
	// so only 2026-10-20 and 2026-10-21 are missing when today is 2026-10-22.
	seedSucceededRange(t, postgres.MinDownloadDate, civil.Date{Year: 2026, Month: 10, Day: 19})

	// With today = 2026-10-22, updateDownloadInfo should process 2026-10-20 and 2026-10-21.
	today := civil.Date{Year: 2026, Month: 10, Day: 22}
	if err := updateDownloadInfo(ctx, testDB, fsys, today); err != nil {
		t.Fatal(err)
	}

	d20 := civil.Date{Year: 2026, Month: 10, Day: 20}
	d21 := civil.Date{Year: 2026, Month: 10, Day: 21}

	gotInfos := readDBDownloads(t)
	wantInfos := []postgres.DownloadInfo{
		{ModulePath: "example.com/a", Date: d20, Downloads: 300, UniqueIPs: 30},
		{ModulePath: "example.com/a", Date: d21, Downloads: 400, UniqueIPs: 40},
		{ModulePath: "example.com/b", Date: d20, Downloads: 50, UniqueIPs: 5},
		{ModulePath: "example.com/b", Date: d21, Downloads: 150, UniqueIPs: 15},
		{ModulePath: "example.com/c", Date: d20, Downloads: 75, UniqueIPs: 8},
	}
	if diff := cmp.Diff(wantInfos, gotInfos); diff != "" {
		t.Errorf("module_downloads mismatch (-want +got):\n%s", diff)
	}

	for _, d := range []civil.Date{d20, d21} {
		status, errMsg := readDBState(t, d)
		if status != http.StatusOK || errMsg != "" {
			t.Errorf("state(%s) = (%d, %q), want (%d, \"\")", d, status, errMsg, http.StatusOK)
		}
	}

	want30d := map[string]int64{
		"example.com/a": 700,
		"example.com/b": 200,
		"example.com/c": 75,
	}
	if diff := cmp.Diff(want30d, readDBDownloads30d(t)); diff != "" {
		t.Errorf("module_downloads_30d mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateDownloadInfoLimit(t *testing.T) {
	ctx := t.Context()
	defer postgres.ResetTestDB(testDB, t)

	today := postgres.MinDownloadDate.AddDays(maxDatesPerRun + 2)
	memFS := fstest.MapFS{}
	for i := range maxDatesPerRun + 2 {
		d := postgres.MinDownloadDate.AddDays(i)
		name := "date=" + d.String() + "/counts-00000-of-00001.csv"
		memFS[name] = &fstest.MapFile{
			Data: []byte(validHeader + "example.com/a,,1,10,1,1\n"),
		}
	}

	if err := updateDownloadInfo(ctx, testDB, memFS, today); err != nil {
		t.Fatal(err)
	}

	// Only the maxDatesPerRun most recent dates should have been processed.
	got := readDBDownloads(t)
	if len(got) != maxDatesPerRun {
		t.Fatalf("got %d rows, want %d", len(got), maxDatesPerRun)
	}
	firstExpected := today.AddDays(-maxDatesPerRun)
	for i := range maxDatesPerRun {
		wantDate := firstExpected.AddDays(i)
		if got[i].Date != wantDate {
			t.Errorf("row %d date = %s, want %s", i, got[i].Date, wantDate)
		}
	}
}

func TestUpdateDownloadInfoOldBackfillSkipsRefresh(t *testing.T) {
	ctx := t.Context()
	defer postgres.ResetTestDB(testDB, t)

	latest := civil.Date{Year: 2026, Month: 10, Day: 20}
	oldGap := latest.AddDays(-30) // outside the 30-day window [latest-29, latest]

	// Populate state rows for all dates except oldGap.
	seedSucceededRange(t, postgres.MinDownloadDate, oldGap.AddDays(-1))
	seedSucceededRange(t, oldGap.AddDays(1), latest)
	if err := testDB.UpdateModuleDownloads30d(ctx); err != nil {
		t.Fatal(err)
	}
	// Insert a download row inside the 30-day window *without* refreshing the view.
	// If updateDownloadInfo skips the refresh (as expected), the view stays empty.
	if err := testDB.UpsertModuleDownloads(ctx, []postgres.DownloadInfo{
		{ModulePath: "example.com/marker", Date: latest, Downloads: 1, UniqueIPs: 1},
	}); err != nil {
		t.Fatal(err)
	}

	memFS := fstest.MapFS{
		"date=" + oldGap.String() + "/counts-00000-of-00001.csv": {
			Data: []byte(validHeader + "example.com/old,,1,50,1,5\n"),
		},
	}
	if err := updateDownloadInfo(ctx, testDB, memFS, latest.AddDays(1)); err != nil {
		t.Fatal(err)
	}
	status, errMsg := readDBState(t, oldGap)
	if status != http.StatusOK || errMsg != "" {
		t.Errorf("state(%s) = (%d, %q), want (%d, \"\")", oldGap, status, errMsg, http.StatusOK)
	}
	if got := readDBDownloads30d(t); len(got) != 0 {
		t.Errorf("module_downloads_30d unexpectedly refreshed: got %v, want empty", got)
	}
}

func seedSucceededRange(t *testing.T, from, to civil.Date) {
	t.Helper()
	const seed = `
		INSERT INTO module_download_states (date, status, error, tries)
		SELECT d::date, $3, '', 1
		FROM generate_series($1::date, $2::date, INTERVAL '1 day') AS d
	`
	if _, err := testDB.Underlying().Exec(t.Context(), seed, from, to, http.StatusOK); err != nil {
		t.Fatal(err)
	}
}

func collectRecords(t *testing.T, fsys fs.FS, date civil.Date) ([]inputRecord, error) {
	t.Helper()
	var got []inputRecord
	for rec, err := range readRecords(t.Context(), fsys, date) {
		if err != nil {
			return nil, err
		}
		got = append(got, rec)
	}
	return got, nil
}

func readDBDownloads(t *testing.T) []postgres.DownloadInfo {
	t.Helper()
	const q = `
		SELECT p.path, md.date, md.downloads, md.unique_ips
		FROM module_downloads md
		JOIN paths p ON p.id = md.module_path_id
		ORDER BY p.path, md.date
	`
	var got []postgres.DownloadInfo
	err := testDB.Underlying().RunQuery(t.Context(), q, func(rows *sql.Rows) error {
		var info postgres.DownloadInfo
		if err := rows.Scan(&info.ModulePath, &info.Date, &info.Downloads, &info.UniqueIPs); err != nil {
			return err
		}
		got = append(got, info)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func readDBDownloads30d(t *testing.T) map[string]int64 {
	t.Helper()
	const q = `
		SELECT p.path, v.downloads_30d
		FROM module_downloads_30d v
		JOIN paths p ON p.id = v.module_path_id
	`
	got := map[string]int64{}
	err := testDB.Underlying().RunQuery(t.Context(), q, func(rows *sql.Rows) error {
		var path string
		var downloads int64
		if err := rows.Scan(&path, &downloads); err != nil {
			return err
		}
		got[path] = downloads
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func readDBState(t *testing.T, d civil.Date) (int, string) {
	t.Helper()
	var (
		status int
		errMsg string
	)
	const q = `SELECT status, error FROM module_download_states WHERE date = $1`
	if err := testDB.Underlying().QueryRow(t.Context(), q, d).Scan(&status, &errMsg); err != nil {
		t.Fatal(err)
	}
	return status, errMsg
}
