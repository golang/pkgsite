// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"strings"

	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// IsExcluded reports whether the path matches the excluded list.
// A path matches an entry on the excluded list if it equals the entry, or
// is a component-wise suffix of the entry.
// So path "bad/ness" matches entries "bad" and "bad/", but path "badness"
// matches neither of those.
func (db *DB) IsExcluded(ctx context.Context, path string) (_ bool, err error) {
	defer derrors.Wrap(&err, "DB.IsExcluded(ctx, %q)", path)

	eps := db.expoller.Current().([]string)
	for _, prefix := range eps {
		prefixSlash := prefix
		if !strings.HasSuffix(prefix, "/") {
			prefixSlash += "/"
		}
		if path == prefix || strings.HasPrefix(path, prefixSlash) {
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
	if err == nil {
		db.expoller.Poll(ctx)
	}
	return err
}

// GetExcludedPrefixes reads all the excluded prefixes from the database.
func (db *DB) GetExcludedPrefixes(ctx context.Context) ([]string, error) {
	return getExcludedPrefixes(ctx, db.db)
}

func getExcludedPrefixes(ctx context.Context, db *database.DB) ([]string, error) {
	return database.Collect1[string](ctx, db, `SELECT prefix FROM excluded_prefixes`)
}
