// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// IsExcluded reports whether the path matches the excluded list.
func (db *DB) IsExcluded(ctx context.Context, path string) (_ bool, err error) {
	defer derrors.Wrap(&err, "DB.IsExcluded(ctx, %q)", path)

	db.ensureExcludedPrefixes(ctx)
	excludedPrefixes.mu.Lock()
	defer excludedPrefixes.mu.Unlock()
	if excludedPrefixes.err != nil {
		return false, excludedPrefixes.err
	}
	for _, prefix := range excludedPrefixes.prefixes {
		if strings.HasPrefix(path, prefix) {
			log.Infof(ctx, "path %q matched excluded prefix %q", path, prefix)
			return true, nil
		}
	}
	return false, nil
}

// InsertExcludedPrefix inserts prefix into the excluded_prefixes table.
//
// For real-time administration (e.g. DOS prevention), use the dbadmin tool.
// to exclude or unexclude a prefix. If the exclusion is permanent (e.g. a user
// request), also add the prefix and reason to the excluded.txt file.
func (db *DB) InsertExcludedPrefix(ctx context.Context, prefix, user, reason string) (err error) {
	defer derrors.Wrap(&err, "DB.InsertExcludedPrefix(ctx, %q, %q)", prefix, reason)

	_, err = db.db.Exec(ctx, "INSERT INTO excluded_prefixes (prefix, created_by, reason) VALUES ($1, $2, $3)",
		prefix, user, reason)
	if err != nil {
		// Arrange to re-read the excluded_prefixes table on the next call to IsExcluded.
		setExcludedPrefixesLastFetched(time.Time{})
	}
	return err
}

// In-memory copy of excluded_prefixes.
var excludedPrefixes struct {
	mu          sync.Mutex
	prefixes    []string
	err         error
	lastFetched time.Time
}

func setExcludedPrefixesLastFetched(t time.Time) {
	excludedPrefixes.mu.Lock()
	excludedPrefixes.lastFetched = t
	excludedPrefixes.mu.Unlock()
}

const excludedPrefixesExpiration = time.Minute

// ensureExcludedPrefixes makes sure the in-memory copy of the
// excluded_prefixes table is up to date.
func (db *DB) ensureExcludedPrefixes(ctx context.Context) {
	excludedPrefixes.mu.Lock()
	lastFetched := excludedPrefixes.lastFetched
	excludedPrefixes.mu.Unlock()
	if time.Since(lastFetched) < excludedPrefixesExpiration {
		return
	}
	prefixes, err := db.GetExcludedPrefixes(ctx)
	excludedPrefixes.mu.Lock()
	defer excludedPrefixes.mu.Unlock()
	excludedPrefixes.lastFetched = time.Now()
	excludedPrefixes.prefixes = prefixes
	excludedPrefixes.err = err
	if err != nil {
		log.Errorf(ctx, "reading excluded_prefixes: %v", err)
	}
}

// GetExcludedPrefixes reads all the excluded prefixes from the database.
func (db *DB) GetExcludedPrefixes(ctx context.Context) ([]string, error) {
	var eps []string
	err := db.db.RunQuery(ctx, `SELECT prefix FROM excluded_prefixes`, func(rows *sql.Rows) error {
		var ep string
		if err := rows.Scan(&ep); err != nil {
			return err
		}
		eps = append(eps, ep)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return eps, nil
}
