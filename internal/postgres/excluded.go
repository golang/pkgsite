// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// IsExcluded reports whether the path matches the excluded list.
func (db *DB) IsExcluded(ctx context.Context, path string) (_ bool, err error) {
	defer derrors.Wrap(&err, "DB.IsExcluded(ctx, %q)", path)

	const query = "SELECT prefix FROM excluded_prefixes WHERE starts_with($1, prefix);"
	row := db.queryRow(ctx, query, path)
	var prefix string
	err = row.Scan(&prefix)
	switch err {
	case nil:
		log.Infof("path %q matched excluded prefix %q", path, prefix)
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}
