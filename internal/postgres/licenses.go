// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

func (db *DB) getLicenses(ctx context.Context, fullPath, modulePath string, unitID int) (_ []*licenses.License, err error) {
	defer derrors.WrapStack(&err, "getLicenses(ctx, %d)", unitID)
	defer middleware.ElapsedStat(ctx, "getLicenses")()

	query := `
		SELECT
			l.types,
			l.file_path,
			l.contents,
			l.coverage
		FROM
			licenses l
		INNER JOIN
			units u
		ON
			u.module_id=l.module_id
		INNER JOIN
			modules m
		ON
			u.module_id=m.id
		WHERE
			u.id = $1;`

	rows, err := db.db.Query(ctx, query, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	moduleLicenses, err := collectLicenses(rows, db.bypassLicenseCheck)
	if err != nil {
		return nil, err
	}

	// The `query` returns all licenses for the module version. We need to
	// filter the licenses that applies to the specified fullPath, i.e.
	// A license in the current or any parent directory of the specified
	// fullPath applies to it.
	var lics []*licenses.License
	for _, l := range moduleLicenses {
		if modulePath == stdlib.ModulePath {
			lics = append(lics, l)
		} else {
			licensePath := path.Join(modulePath, path.Dir(l.FilePath))
			if strings.HasPrefix(fullPath, licensePath) {
				lics = append(lics, l)
			}
		}
	}
	if !db.bypassLicenseCheck {
		for _, l := range lics {
			l.RemoveNonRedistributableData()
		}
	}
	return lics, nil
}

// getModuleLicenses returns all licenses associated with the given module path and
// version. These are the top-level licenses in the module zip file.
// It returns an InvalidArgument error if the module path or version is invalid.
func (db *DB) getModuleLicenses(ctx context.Context, moduleID int) (_ []*licenses.License, err error) {
	defer derrors.WrapStack(&err, "getModuleLicenses(ctx, %d)", moduleID)

	query := `
	SELECT
		types, file_path, contents, coverage
	FROM
		licenses
	WHERE
		module_id = $1 AND position('/' in file_path) = 0
    `
	rows, err := db.db.Query(ctx, query, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectLicenses(rows, db.bypassLicenseCheck)
}

// collectLicenses converts the sql rows to a list of licenses. The columns
// must be types, file_path and contents, in that order.
func collectLicenses(rows *sql.Rows, bypassLicenseCheck bool) ([]*licenses.License, error) {
	mustHaveColumns(rows, "types", "file_path", "contents", "coverage")
	var lics []*licenses.License
	for rows.Next() {
		var (
			lic          = &licenses.License{Metadata: &licenses.Metadata{}}
			licenseTypes []string
			covBytes     []byte
		)
		if err := rows.Scan(pq.Array(&licenseTypes), &lic.FilePath, &lic.Contents, &covBytes); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		// The coverage column is JSON for either the new or old
		// licensecheck.Coverage struct. The new Match type has an ID field
		// which is always populated, but the old one doesn't. First try
		// unmarshalling the new one, then if that doesn't populate the ID
		// field, try the old.
		if err := json.Unmarshal(covBytes, &lic.Coverage); err != nil {
			return nil, err
		}
		lic.Types = licenseTypes
		if !bypassLicenseCheck {
			lic.RemoveNonRedistributableData()
		}
		lics = append(lics, lic)
	}
	sort.Slice(lics, func(i, j int) bool {
		return compareLicenses(lics[i].Metadata, lics[j].Metadata)
	})
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return lics, nil
}

// mustHaveColumns panics if the columns of rows does not match wantColumns.
func mustHaveColumns(rows *sql.Rows, wantColumns ...string) {
	gotColumns, err := rows.Columns()
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(gotColumns, wantColumns) {
		panic(fmt.Sprintf("got columns %v, want $%v", gotColumns, wantColumns))
	}
}

// zipLicenseMetadata constructs licenses.Metadata from the given license types
// and paths, by zipping and then sorting.
func zipLicenseMetadata(licenseTypes []string, licensePaths []string) (_ []*licenses.Metadata, err error) {
	defer derrors.WrapStack(&err, "zipLicenseMetadata(%v, %v)", licenseTypes, licensePaths)

	if len(licenseTypes) != len(licensePaths) {
		return nil, fmt.Errorf("BUG: got %d license types and %d license paths", len(licenseTypes), len(licensePaths))
	}
	byPath := make(map[string]*licenses.Metadata)
	var mds []*licenses.Metadata
	for i, p := range licensePaths {
		md, ok := byPath[p]
		if !ok {
			md = &licenses.Metadata{FilePath: p}
			mds = append(mds, md)
		}
		// By convention, we insert a license path with empty corresponding license
		// type if we are unable to detect *any* licenses in the file. This ensures
		// that we mark this package as non-redistributable.
		if licenseTypes[i] != "" {
			md.Types = append(md.Types, licenseTypes[i])
		}
	}
	sort.Slice(mds, func(i, j int) bool {
		return compareLicenses(mds[i], mds[j])
	})
	return mds, nil
}

// compareLicenses reports whether i < j according to our license sorting
// semantics.
func compareLicenses(i, j *licenses.Metadata) bool {
	if len(strings.Split(i.FilePath, "/")) > len(strings.Split(j.FilePath, "/")) {
		return true
	}
	return i.FilePath < j.FilePath
}
