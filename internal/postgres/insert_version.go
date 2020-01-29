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
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/version"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// InsertVersion inserts a version into the database using
// db.saveVersion, along with a search document corresponding to each of its
// packages.
func (db *DB) InsertVersion(ctx context.Context, v *internal.Version) (err error) {
	defer func() {
		if v == nil {
			derrors.Wrap(&err, "DB.InsertVersion(ctx, nil)")
		} else {
			derrors.Wrap(&err, "DB.InsertVersion(ctx, Version(%q, %q))", v.ModulePath, v.Version)
		}
	}()

	if err := validateVersion(v); err != nil {
		return fmt.Errorf("validateVersion: %v: %w", err, derrors.InvalidArgument)
	}
	removeNonDistributableData(v)

	if err := db.saveVersion(ctx, v); err != nil {
		return err
	}

	// If there is a more recent version of this module that has an alternative
	// module path, then do not insert its packages into search_documents. This
	// happens when a module that initially does not have a go.mod file is
	// forked or fetched via some non-canonical path (such as an alternative
	// capitalization), and then in a later version acquires a go.mod file.
	//
	// To take an actual example: github.com/sirupsen/logrus@v1.1.0 has a go.mod
	// file that establishes that path as canonical. But v1.0.6 does not have a
	// go.mod file. So the miscapitalized path github.com/Sirupsen/logrus at
	// v1.1.0 is marked as an alternative path (code 491) by
	// internal/fetch.FetchVersion and is not inserted into the DB, but at
	// v1.0.6 it is considered valid, and we end up here. We still insert
	// github.com/Sirupsen/logrus@v1.0.6 in the versions table and friends so
	// that users who import it can find information about it, but we don't want
	// it showing up in search results.
	//
	// Note that we end up here only if we first saw the alternative version
	// (github.com/Sirupsen/logrus@v1.1.0 in the example) and then see the valid
	// one. The "if code == 491" section of internal/etl.fetchAndUpdateState
	// handles the case where we fetch the versions in the other order.
	row := db.db.QueryRow(ctx, `
			SELECT 1 FROM module_version_states
			WHERE module_path = $1 AND sort_version > $2 and status = 491`,
		v.ModulePath, version.ForSorting(v.Version))
	var x int
	if err := row.Scan(&x); err != sql.ErrNoRows {
		log.Infof(ctx, "%s@%s: not inserting into search documents", v.ModulePath, v.Version)
		return err
	}

	// Insert the module's packages into search_documents.
	for _, pkg := range v.Packages {
		if err := db.UpsertSearchDocument(ctx, pkg.Path); err != nil && !errors.Is(err, derrors.InvalidArgument) {
			return err
		}
	}
	return nil
}

// saveVersion inserts a Version into the database along with its packages,
// imports, and licenses.  If any of these rows already exist, the version and
// corresponding will be deleted and reinserted.
// If the version is malformed then insertion will fail.
//
// A derrors.InvalidArgument error will be returned if the given version and
// licenses are invalid.
func (db *DB) saveVersion(ctx context.Context, v *internal.Version) error {
	if v.ReadmeContents == internal.StringFieldMissing {
		return errors.New("saveVersion: version missing ReadmeContents")
	}
	// Sort to ensure proper lock ordering, avoiding deadlocks. See
	// b/141164828#comment8. The only deadlocks we've actually seen are on
	// imports_unique, because they can occur when processing two versions of
	// the same module, which happens regularly. But if we were ever to process
	// the same module and version twice, we could see deadlocks in the other
	// bulk inserts.
	sort.Slice(v.Packages, func(i, j int) bool {
		return v.Packages[i].Path < v.Packages[j].Path
	})
	sort.Slice(v.Licenses, func(i, j int) bool {
		return v.Licenses[i].FilePath < v.Licenses[j].FilePath
	})
	for _, p := range v.Packages {
		sort.Strings(p.Imports)
	}

	err := db.db.Transact(func(tx *sql.Tx) error {
		// If the version exists, delete it to force an overwrite. This allows us
		// to selectively repopulate data after a code change.
		if err := db.DeleteVersion(ctx, tx, v.ModulePath, v.Version); err != nil {
			return fmt.Errorf("error deleting existing versions: %v", err)
		}

		sourceInfoJSON, err := json.Marshal(v.SourceInfo)
		if err != nil {
			return err
		}
		if _, err := database.ExecTx(ctx, tx,
			`INSERT INTO versions(
				module_path,
				version,
				commit_time,
				readme_file_path,
				readme_contents,
				sort_version,
				version_type,
				series_path,
				source_info,
				redistributable,
				has_go_mod)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, $11) ON CONFLICT DO NOTHING`,
			v.ModulePath,
			v.Version,
			v.CommitTime,
			v.ReadmeFilePath,
			v.ReadmeContents,
			version.ForSorting(v.Version),
			v.VersionType,
			v.SeriesPath(),
			sourceInfoJSON,
			v.IsRedistributable,
			v.HasGoMod,
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		var licenseValues []interface{}
		for _, l := range v.Licenses {
			covJSON, err := json.Marshal(l.Coverage)
			if err != nil {
				return fmt.Errorf("marshalling %+v: %v", l.Coverage, err)
			}
			licenseValues = append(licenseValues, v.ModulePath, v.Version,
				l.FilePath, makeValidUnicode(l.Contents), pq.Array(l.Types), covJSON)
		}
		if len(licenseValues) > 0 {
			licenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"contents",
				"types",
				"coverage",
			}
			if err := database.BulkInsert(ctx, tx, "licenses", licenseCols, licenseValues,
				database.OnConflictDoNothing); err != nil {
				return err
			}
		}

		var pkgValues, importValues, importUniqueValues []interface{}
		for _, p := range v.Packages {
			if p.DocumentationHTML == internal.StringFieldMissing {
				return errors.New("saveVersion: package missing DocumentationHTML")
			}
			var licenseTypes, licensePaths []string
			for _, l := range p.Licenses {
				if len(l.Types) == 0 {
					// If a license file has no detected license types, we still need to
					// record it as applicable to the package, because we want to fail
					// closed (meaning if there is a LICENSE file containing unknown
					// licenses, we assume them not to be permissive of redistribution.)
					licenseTypes = append(licenseTypes, "")
					licensePaths = append(licensePaths, l.FilePath)
				} else {
					for _, typ := range l.Types {
						licenseTypes = append(licenseTypes, typ)
						licensePaths = append(licensePaths, l.FilePath)
					}
				}
			}
			pkgValues = append(pkgValues,
				p.Path,
				p.Synopsis,
				p.Name,
				v.Version,
				v.ModulePath,
				p.V1Path,
				p.IsRedistributable,
				p.DocumentationHTML,
				pq.Array(licenseTypes),
				pq.Array(licensePaths),
				p.GOOS,
				p.GOARCH,
				v.CommitTime,
			)
			for _, i := range p.Imports {
				importValues = append(importValues, p.Path, v.ModulePath, v.Version, i)
				importUniqueValues = append(importUniqueValues, p.Path, v.ModulePath, i)
			}
		}
		if len(pkgValues) > 0 {
			pkgCols := []string{
				"path",
				"synopsis",
				"name",
				"version",
				"module_path",
				"v1_path",
				"redistributable",
				"documentation",
				"license_types",
				"license_paths",
				"goos",
				"goarch",
				"commit_time",
			}
			if err := database.BulkInsert(ctx, tx, "packages", pkgCols, pkgValues, database.OnConflictDoNothing); err != nil {
				return err
			}
		}

		if len(importValues) > 0 {
			importCols := []string{
				"from_path",
				"from_module_path",
				"from_version",
				"to_path",
			}
			if err := database.BulkInsert(ctx, tx, "imports", importCols, importValues, database.OnConflictDoNothing); err != nil {
				return err
			}

			importUniqueCols := []string{
				"from_path",
				"from_module_path",
				"to_path",
			}
			if err := database.BulkInsert(ctx, tx, "imports_unique", importUniqueCols, importUniqueValues, database.OnConflictDoNothing); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("DB.saveVersion(ctx, Version(%q, %q)): %w", v.ModulePath, v.Version, err)
	}
	return nil
}

// validateVersion checks that fields needed to insert a version into the
// database are present. Otherwise, it returns an error listing the reasons the
// version cannot be inserted.
func validateVersion(v *internal.Version) error {
	if v == nil {
		return fmt.Errorf("nil version")
	}

	var errReasons []string
	if !utf8.ValidString(v.ReadmeContents) {
		errReasons = append(errReasons, fmt.Sprintf("readme %q is not valid UTF-8", v.ReadmeFilePath))
	}
	for _, l := range v.Licenses {
		if !utf8.ValidString(string(l.Contents)) {
			errReasons = append(errReasons, fmt.Sprintf("license %q contains invalid UTF-8", l.FilePath))
		}
	}
	if v.Version == "" {
		errReasons = append(errReasons, "no specified version")
	}
	if v.ModulePath == "" {
		errReasons = append(errReasons, "no module path")
	}
	if v.ModulePath != stdlib.ModulePath {
		if err := module.CheckPath(v.ModulePath); err != nil {
			errReasons = append(errReasons, fmt.Sprintf("invalid module path (%s)", err))
		}
		if !semver.IsValid(v.Version) {
			errReasons = append(errReasons, "invalid version")
		}
	}
	if len(v.Packages) == 0 {
		errReasons = append(errReasons, "module does not have any packages")
	}
	if v.CommitTime.IsZero() {
		errReasons = append(errReasons, "empty commit time")
	}
	if len(errReasons) == 0 {
		return nil
	}
	return fmt.Errorf("cannot insert version %q: %s", v.Version, strings.Join(errReasons, ", "))
}

// removeNonDistributableData removes any information from the version payload,
// after checking licenses.
func removeNonDistributableData(v *internal.Version) {
	for _, p := range v.Packages {
		if !p.IsRedistributable {
			// Prune derived information that can't be stored.
			p.Synopsis = ""
			p.DocumentationHTML = ""
		}
	}
	if !v.IsRedistributable {
		v.ReadmeFilePath = ""
		v.ReadmeContents = ""
	}
}

// DeleteVersion deletes a Version from the database.
// If tx is non-nil, it will be used to execute the statement.
// Otherwise the statement will be run outside of a transaction.
func (db *DB) DeleteVersion(ctx context.Context, tx *sql.Tx, modulePath, version string) (err error) {
	defer derrors.Wrap(&err, "DB.DeleteVersion(ctx, tx, %q, %q)", modulePath, version)

	// We only need to delete from the versions table. Thanks to ON DELETE
	// CASCADE constraints, that will trigger deletions from all other tables.
	const stmt = `DELETE FROM versions WHERE module_path=$1 AND version=$2`
	if tx == nil {
		_, err = db.db.Exec(ctx, stmt, modulePath, version)
	} else {
		_, err = database.ExecTx(ctx, tx, stmt, modulePath, version)
	}
	return err
}

// makeValidUnicode removes null runes from license contents, because pq doesn't like them.
func makeValidUnicode(bs []byte) string {
	s := string(bs)
	var b strings.Builder
	for _, r := range s {
		if r != 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
