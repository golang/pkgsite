// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
)

// GetStdlibPathsWithSuffix returns information about all paths in the latest version of the standard
// library whose last component is suffix. A path that exactly match suffix is not included;
// the path must end with "/" + suffix.
//
// We are only interested in actual standard library packages: not commands, which we happen to include
// in the stdlib module, and not directories (paths that do not contain a package).
func (db *DB) GetStdlibPathsWithSuffix(ctx context.Context, suffix string) (paths []string, err error) {
	defer derrors.WrapStack(&err, "DB.GetStdlibPaths(ctx, %q)", suffix)

	q := `
		SELECT p.path
		FROM units u
		INNER JOIN paths p
		ON p.id = u.path_id
		WHERE module_id = (
			-- latest release version of stdlib
			SELECT id
			FROM modules
			WHERE module_path = $1
			ORDER BY
				version_type = 'release' DESC,
				sort_version DESC
			LIMIT 1)
			AND u.name != ''
			AND p.path NOT LIKE 'cmd/%'
			AND p.path LIKE '%/' || $2
		ORDER BY p.path
	`
	return collectStrings(ctx, db.db, q, stdlib.ModulePath, suffix)
}
