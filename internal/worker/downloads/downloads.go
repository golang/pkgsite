// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package downloads ingests module download counts into the database.
package downloads

import (
	"cmp"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/worker/downloads/gcsfs"
)

// Update processes all updateable download dates up to yesterday.
func Update(ctx context.Context, db *postgres.DB, gcsURL string) (err error) {
	defer derrors.Wrap(&err, "downloads.Update(ctx, db, %q)", gcsURL)

	rest, ok := strings.CutPrefix(gcsURL, "gs://")
	if !ok {
		return fmt.Errorf("bad GCS URL %q: must start with gs://", gcsURL)
	}
	bucket, prefix, _ := strings.Cut(rest, "/")
	if bucket == "" {
		return fmt.Errorf("bad GCS URL %q: missing bucket", gcsURL)
	}
	gfs, err := gcsfs.New(ctx, bucket)
	if err != nil {
		return err
	}
	defer gfs.Close()

	var fsys fs.FS = gfs
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		fsys, err = fs.Sub(gfs, prefix)
		if err != nil {
			return err
		}
	}

	today := civil.DateOf(time.Now().UTC())
	return updateDownloadInfo(ctx, db, fsys, today)
}

// maxDatesPerRun caps the number of dates processed in a single updateDownloadInfo call.
const maxDatesPerRun = 10

// updateDownloadInfo processes up to maxDatesPerRun updateable download dates,
// starting with the most recent.
func updateDownloadInfo(ctx context.Context, db *postgres.DB, fsys fs.FS, today civil.Date) (err error) {
	defer derrors.Wrap(&err, "updateDownloadInfo(ctx, db, fsys, %s)", today)

	dates, err := db.UpdateableDownloadDates(ctx, today)
	if err != nil {
		return err
	}
	if len(dates) > maxDatesPerRun {
		dates = dates[:maxDatesPerRun]
	}
	var (
		errs       []error
		maxUpdated civil.Date
	)
	for _, d := range dates {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := updateDownloadInfoForDate(ctx, db, fsys, d); err != nil {
			errs = append(errs, err)
			if ctx.Err() != nil {
				return errors.Join(errs...)
			}
		} else if maxUpdated.IsZero() || d.After(maxUpdated) {
			maxUpdated = d
		}
	}
	// Refresh the 30-day materialized view if any updated date falls in the
	// 30-day window ending at the latest successful date.
	if !maxUpdated.IsZero() {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		latest, ok, err := db.LatestDownloadDate(ctx)
		if err != nil {
			errs = append(errs, err)
		} else if ok && !maxUpdated.Before(latest.AddDays(-29)) {
			if err := db.UpdateModuleDownloads30d(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// updateDownloadInfoForDate reads download counts for date from fsys, writes them
// to the module_downloads table, and updates the module_download_states table.
func updateDownloadInfoForDate(ctx context.Context, db *postgres.DB, fsys fs.FS, date civil.Date) (err error) {
	defer derrors.Wrap(&err, "updateDownloadInfoForDate(ctx, db, fsys, %s)", date)

	log.Infof(ctx, "updating download info for %s", date)

	run := func() (int, error) {
		infos, err := readDownloadInfo(ctx, fsys, date)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return http.StatusNotFound, err
			}
			return derrors.ToStatus(err), err
		}
		if len(infos) == 0 {
			return http.StatusNotFound, errors.New("no download records")
		}
		if err := db.UpsertModuleDownloads(ctx, infos); err != nil {
			return derrors.ToStatus(err), err
		}
		return http.StatusOK, nil
	}

	status, runErr := run()
	// If the context was canceled, don't write a failure state (which would delay
	// retries) or attempt a DB call with an already-canceled context.
	if err := ctx.Err(); err != nil {
		if runErr != nil {
			return runErr
		}
		return err
	}
	var errMsg string
	if runErr != nil {
		errMsg = runErr.Error()
	}
	stateErr := db.UpsertModuleDownloadState(ctx, date, status, errMsg)
	return errors.Join(runErr, stateErr)
}

// readDownloadInfo reads all CSV shards in fsys for date and returns the
// resulting DownloadInfo slice sorted by ModulePath.
// Only pre-aggregated rows (with an empty version) are included; duplicate
// module paths return an error.
func readDownloadInfo(ctx context.Context, fsys fs.FS, date civil.Date) ([]postgres.DownloadInfo, error) {
	// Read records and check for duplicate module paths.
	seen := map[string]bool{}
	var dls []postgres.DownloadInfo
	for rec, err := range readRecords(ctx, fsys, date) {
		if err != nil {
			return nil, err
		}
		if seen[rec.modulePath] {
			return nil, fmt.Errorf("readDownloadInfo: %s: duplicate module path %q", date, rec.modulePath)
		}
		// encoding/csv slices every field out of one backing string per row.
		// Clone modulePath once so seen and dls don't pin the entire CSV row in memory.
		mp := strings.Clone(rec.modulePath)
		seen[mp] = true
		dls = append(dls, postgres.DownloadInfo{
			ModulePath: mp,
			Date:       date,
			Downloads:  rec.zipCount,
			UniqueIPs:  rec.zipUniqueIPs,
		})
	}
	slices.SortFunc(dls, func(a, b postgres.DownloadInfo) int {
		return cmp.Compare(a.ModulePath, b.ModulePath)
	})
	return dls, nil
}

// inputRecord holds a single row from a download counts CSV file.
// It has no version field because only rows with an empty version are kept.
type inputRecord struct {
	modulePath   string
	zipCount     int64
	zipUniqueIPs int64
}

var csvColumns = []string{"module", "version", "mod_count", "zip_count", "mod_unique_ips", "zip_unique_ips"}

const versionColumnIndex = 1

// readRecords reads all CSV shards in fsys for the given date
// from directory "date=YYYY-MM-DD" and yields each inputRecord.
func readRecords(ctx context.Context, fsys fs.FS, date civil.Date) iter.Seq2[inputRecord, error] {
	return func(yield func(inputRecord, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(inputRecord{}, err)
			return
		}
		dir := "date=" + date.String()
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			yield(inputRecord{}, err)
			return
		}

		var shards []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".csv") {
				shards = append(shards, e.Name())
			}
		}
		// fs.ReadDir returns fsys.ReadDir directly without re-sorting when fsys
		// implements fs.ReadDirFS; sort defensively in case a custom FS does not.
		slices.Sort(shards)
		if err := validateShards(dir, shards); err != nil {
			yield(inputRecord{}, err)
			return
		}

		for _, name := range shards {
			if err := ctx.Err(); err != nil {
				yield(inputRecord{}, err)
				return
			}
			filePath := path.Join(dir, name)
			if !readShard(fsys, filePath, yield) {
				return
			}
		}
	}
}

// validateShards checks that shards is a complete, sorted sequence counts-00000-of-NNNNN.csv
// through counts-(N-1)-of-NNNNN.csv.
func validateShards(dir string, shards []string) error {
	if len(shards) == 0 {
		return fmt.Errorf("no CSV shards in %s", dir)
	}
	// Parse the expected total from the first shard filename so that missing-shard
	// errors report e.g. "got 1 shards, want 2" rather than expecting "-of-00001".
	var total int
	if rest, ok := strings.CutPrefix(shards[0], "counts-"); ok {
		if base, ok := strings.CutSuffix(rest, ".csv"); ok {
			if _, totStr, ok := strings.Cut(base, "-of-"); ok {
				if n, err := strconv.Atoi(totStr); err == nil && n > 0 {
					total = n
				}
			}
		}
	}
	if total == 0 {
		return fmt.Errorf("in %s: unexpected shard filename %q", dir, shards[0])
	}
	if len(shards) != total {
		return fmt.Errorf("in %s: got %d shards, want %d (from %q)", dir, len(shards), total, shards[0])
	}
	for i, got := range shards {
		want := fmt.Sprintf("counts-%05d-of-%05d.csv", i, total)
		if got != want {
			return fmt.Errorf("in %s: shard %d is %q, want %q", dir, i, got, want)
		}
	}
	return nil
}

// readShard reads a single CSV shard file and yields its records.
// It reads only records with version="", which are aggregated over all versions.
// It returns false if iteration should stop (either because yield returned false or an error occurred).
func readShard(fsys fs.FS, filePath string, yield func(inputRecord, error) bool) bool {
	f, err := fsys.Open(filePath)
	if err != nil {
		yield(inputRecord{}, err)
		return false
	}
	defer f.Close()

	cr := csv.NewReader(f)
	// Reuse the same backing slice across Read calls to avoid allocating a new
	// slice on every CSV row; row values are copied or parsed before the next Read.
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err != nil {
		yield(inputRecord{}, fmt.Errorf("%s: reading header: %w", filePath, err))
		return false
	}
	if !slices.Equal(header, csvColumns) {
		yield(inputRecord{}, fmt.Errorf("%s: got header %v, want %v", filePath, header, csvColumns))
		return false
	}
	// Set FieldsPerRecord after validating the header so a header with the wrong
	// column count produces the clearer mismatch error above instead of ErrFieldCount.
	cr.FieldsPerRecord = len(csvColumns)

	for {
		row, err := cr.Read()
		if err == io.EOF {
			return true
		}
		if err != nil {
			yield(inputRecord{}, fmt.Errorf("%s: %w", filePath, err))
			return false
		}
		// Read only rows with empty version.
		if row[versionColumnIndex] != "" {
			continue
		}
		rec, err := parseRecord(row)
		if err != nil {
			line, _ := cr.FieldPos(0)
			yield(inputRecord{}, fmt.Errorf("%s:%d: %w", filePath, line, err))
			return false
		}
		if !yield(rec, nil) {
			return false
		}
	}
}

func parseRecord(row []string) (inputRecord, error) {
	if strings.TrimSpace(row[0]) == "" {
		return inputRecord{}, errors.New("empty module path")
	}
	// row[2] (mod_count) and row[4] (mod_unique_ips) are ignored.
	zipCount, err := parseNonNegativeInt("zip_count", row[3])
	if err != nil {
		return inputRecord{}, err
	}
	zipUniqueIPs, err := parseNonNegativeInt("zip_unique_ips", row[5])
	if err != nil {
		return inputRecord{}, err
	}
	return inputRecord{
		modulePath:   row[0],
		zipCount:     zipCount,
		zipUniqueIPs: zipUniqueIPs,
	}, nil
}

func parseNonNegativeInt(col, val string) (int64, error) {
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", col, val, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative %s %d", col, n)
	}
	return n, nil
}
