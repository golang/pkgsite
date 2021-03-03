// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
)

// GetNestedModules returns the latest major version of all nested modules
// given a modulePath path prefix with or without major version.
func (db *DB) GetNestedModules(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.WrapStack(&err, "GetNestedModules(ctx, %v)", modulePath)
	defer middleware.ElapsedStat(ctx, "GetNestedModules")()

	query := `
		SELECT DISTINCT ON (series_path)
			m.module_path,
			m.version,
			m.commit_time,
			m.redistributable,
			m.has_go_mod,
			m.deprecated_comment,
			m.source_info
		FROM
			modules m
		WHERE
			m.module_path LIKE $1 || '/%'
		ORDER BY
			m.series_path,
			m.incompatible,
			m.version_type = 'release' DESC,
			m.sort_version DESC;
	`

	var modules []*internal.ModuleInfo
	collect := func(rows *sql.Rows) error {
		mi, err := scanModuleInfo(rows.Scan)
		if err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		isExcluded, err := db.IsExcluded(ctx, mi.ModulePath)
		if err != nil {
			return err
		}
		if !isExcluded {
			modules = append(modules, mi)
		}
		return nil
	}
	seriesPath := internal.SeriesPathForModule(modulePath)
	if err := db.db.RunQuery(ctx, query, collect, seriesPath); err != nil {
		return nil, err
	}

	if err := populateLatestInfos(ctx, db, modules); err != nil {
		return nil, err
	}

	return modules, nil
}

// GetImportedBy fetches and returns all of the packages that import the
// package with path.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
//
// Instead of supporting pagination, this query runs with a limit.
func (db *DB) GetImportedBy(ctx context.Context, pkgPath, modulePath string, limit int) (paths []string, err error) {
	defer derrors.WrapStack(&err, "GetImportedBy(ctx, %q, %q)", pkgPath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetImportedBy")()

	if pkgPath == "" {
		return nil, fmt.Errorf("pkgPath cannot be empty: %w", derrors.InvalidArgument)
	}
	query := `
		SELECT
			DISTINCT from_path
		FROM
			imports_unique
		WHERE
			to_path = $1
		AND
			from_module_path <> $2
		ORDER BY
			from_path
		LIMIT $3`

	var importedby []string
	collect := func(rows *sql.Rows) error {
		var fromPath string
		if err := rows.Scan(&fromPath); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		importedby = append(importedby, fromPath)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, pkgPath, modulePath, limit); err != nil {
		return nil, err
	}
	return importedby, nil
}

// GetImportedByCount returns the number of packages that import pkgPath.
func (db *DB) GetImportedByCount(ctx context.Context, pkgPath, modulePath string) (_ int, err error) {
	defer derrors.WrapStack(&err, "GetImportedByCount(ctx, %q, %q)", pkgPath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetImportedByCount")()

	if pkgPath == "" {
		return 0, fmt.Errorf("pkgPath cannot be empty: %w", derrors.InvalidArgument)
	}
	query := `
		SELECT imported_by_count
		FROM
			search_documents
		WHERE
			package_path = $1
	`
	var n int
	err = db.db.QueryRow(ctx, query, pkgPath).Scan(&n)
	switch err {
	case sql.ErrNoRows:
		return 0, nil
	case nil:
		return n, nil
	default:
		return 0, err
	}
}

// GetModuleInfo fetches a module version from the database with the primary key
// (module_path, version).
func (db *DB) GetModuleInfo(ctx context.Context, modulePath, resolvedVersion string) (_ *internal.ModuleInfo, err error) {
	defer derrors.WrapStack(&err, "GetModuleInfo(ctx, %q, %q)", modulePath, resolvedVersion)

	query := `
		SELECT
			module_path,
			version,
			commit_time,
			redistributable,
			has_go_mod,
			deprecated_comment,
			source_info
		FROM
			modules
		WHERE
			module_path = $1
			AND version = $2;`

	row := db.db.QueryRow(ctx, query, modulePath, resolvedVersion)
	mi, err := scanModuleInfo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, derrors.NotFound
	}
	if err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	if err := populateLatestInfo(ctx, db, mi); err != nil {
		return nil, err
	}
	return mi, nil
}

// jsonbScanner scans a jsonb value into a Go value.
type jsonbScanner struct {
	ptr interface{} // a pointer to a Go struct or other JSON-serializable value
}

func (s jsonbScanner) Scan(value interface{}) (err error) {
	defer derrors.Wrap(&err, "jsonbScanner(%+v)", value)

	vptr := reflect.ValueOf(s.ptr)
	if value == nil {
		// *s.ptr = nil
		vptr.Elem().Set(reflect.Zero(vptr.Elem().Type()))
		return nil
	}
	jsonBytes, ok := value.([]byte)
	if !ok {
		return errors.New("not a []byte")
	}
	// v := &[type of *s.ptr]
	v := reflect.New(vptr.Elem().Type())
	if err := json.Unmarshal(jsonBytes, v.Interface()); err != nil {
		return err
	}

	// *s.ptr = *v
	vptr.Elem().Set(v.Elem())
	return nil
}

// scanModuleInfo constructs an *internal.ModuleInfo from the given scanner.
func scanModuleInfo(scan func(dest ...interface{}) error) (*internal.ModuleInfo, error) {
	var (
		mi         internal.ModuleInfo
		depComment *string
	)
	if err := scan(&mi.ModulePath, &mi.Version, &mi.CommitTime,
		&mi.IsRedistributable, &mi.HasGoMod, &depComment, jsonbScanner{&mi.SourceInfo}); err != nil {
		return nil, err
	}
	if depComment != nil {
		mi.Deprecated = true
		mi.DeprecationComment = *depComment
	}
	return &mi, nil
}

// packageLinkRegexp matches cross-package identifier links that have been
// generated by the dochtml package. At the time this hack was added, these
// links are all constructed to have either the form
//   <a href="/pkg/[path]">[name]</a>
// or the form
//   <a href="/pkg/[path]#identifier">[name]</a>
//
// The packageLinkRegexp mutates these links as follows:
//   - remove the now unnecessary '/pkg' path prefix
var packageLinkRegexp = regexp.MustCompile(`(<a href="/)pkg/([^?#"]+)((?:#[^"]*)?">.*?</a>)`)

// removePkgPrefix removes the /pkg path prefix from links in docHTML.
// See documentation for packageLinkRegexp for explanation and
// TestRemovePkgPrefix for examples. It preserves the safety of its argument.
// That is, if docHTML is safe from XSS attacks, so is
// removePkgPrefix(docHTML).
//
// Although we don't add "/pkg" to links after https://golang.org/cl/259101,
// do not remove this function until all databases have been reprocessed.
func removePkgPrefix(docHTML string) string {
	return packageLinkRegexp.ReplaceAllString(docHTML, `$1$2$3`)
}
