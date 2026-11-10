// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package downloads

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"cloud.google.com/go/civil"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/postgres"
)

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
