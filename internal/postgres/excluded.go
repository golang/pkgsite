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

// IsExcluded reports whether the path and version matches the excluded list.
// A path@version matches an entry on the excluded list if it equals the entry, or
// if the pattern has no version and the path is a component-wise suffix of it.
// So path "bad/ness" matches entries "bad" and "bad/", but path "badness"
// matches neither of those.
func (db *DB) IsExcluded(ctx context.Context, path, version string) bool {
	eps := db.expoller.Current().([]string)
	for _, pattern := range eps {
		if excludes(pattern, path, version) {
			log.Infof(ctx, "path %q and version %q matched excluded pattern %q", path, version, pattern)
			return true
		}
	}
	return false
}

func excludes(pattern, path, version string) bool {
	// Patterns with "@" must match exactly.
	mod, ver, found := strings.Cut(pattern, "@")
	if found {
		return mod == path && ver == version
	}
	// Patterns without "@" can match exactly or be a componentwise prefix.
	if pattern == path {
		return true
	}
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	return strings.HasPrefix(path, pattern)
}

// InsertExcludedPattern inserts pattern into the excluded_prefixes table.
// The pattern may be a module path prefix, or of the form module@version.
//
// For real-time administration (e.g. DOS prevention), use the dbadmin tool.
// to exclude or unexclude a prefix. If the exclusion is permanent (e.g. a user
// request), also add the pattern and reason to the excluded.txt file.
func (db *DB) InsertExcludedPattern(ctx context.Context, pattern, user, reason string) (err error) {
	defer derrors.Wrap(&err, "DB.InsertExcludedPattern(ctx, %q, %q)", pattern, reason)

	_, err = db.db.Exec(ctx, "INSERT INTO excluded_prefixes (prefix, created_by, reason) VALUES ($1, $2, $3)",
		pattern, user, reason)
	if err == nil {
		db.expoller.Poll(ctx)
	}
	return err
}

// GetExcludedPatterns reads all the excluded prefixes from the database.
func (db *DB) GetExcludedPatterns(ctx context.Context) ([]string, error) {
	return getExcludedPatterns(ctx, db.db)
}

func getExcludedPatterns(ctx context.Context, db *database.DB) ([]string, error) {
	return database.Collect1[string](ctx, db, `SELECT prefix FROM excluded_prefixes`)
}
