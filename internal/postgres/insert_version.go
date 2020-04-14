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

// InsertModule inserts a version into the database using
// db.saveVersion, along with a search document corresponding to each of its
// packages.
func (db *DB) InsertModule(ctx context.Context, m *internal.Module) (err error) {
	defer func() {
		if m == nil {
			derrors.Wrap(&err, "DB.InsertModule(ctx, nil)")
		} else {
			derrors.Wrap(&err, "DB.InsertModule(ctx, Version(%q, %q))", m.ModulePath, m.Version)
		}
	}()

	if err := validateModule(m); err != nil {
		return fmt.Errorf("validateVersion: %v: %w", err, derrors.InvalidArgument)
	}
	removeNonDistributableData(m)

	if err := db.saveModule(ctx, m); err != nil {
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
	// one. The "if code == 491" section of internal/worker.fetchAndUpdateState
	// handles the case where we fetch the versions in the other order.
	row := db.db.QueryRow(ctx, `
			SELECT 1 FROM module_version_states
			WHERE module_path = $1 AND sort_version > $2 and status = 491`,
		m.ModulePath, version.ForSorting(m.Version))
	var x int
	if err := row.Scan(&x); err != sql.ErrNoRows {
		log.Infof(ctx, "%s@%s: not inserting into search documents", m.ModulePath, m.Version)
		return err
	}

	// Insert the module's packages into search_documents.
	return db.UpsertSearchDocuments(ctx, m)
}

// saveModule inserts a Module into the database along with its packages,
// imports, and licenses.  If any of these rows already exist, the module and
// corresponding will be deleted and reinserted.
// If the module is malformed then insertion will fail.
//
// A derrors.InvalidArgument error will be returned if the given module and
// licenses are invalid.
func (db *DB) saveModule(ctx context.Context, m *internal.Module) (err error) {
	derrors.Wrap(&err, "DB.saveModule(ctx, Version(%q, %q))", m.ModulePath, m.Version)
	if m.ReadmeContents == internal.StringFieldMissing {
		// We don't expect this to ever happen here, but checking just in case.
		return errors.New("saveModule: version missing ReadmeContents")
	}
	// Sort to ensure proper lock ordering, avoiding deadlocks. See
	// b/141164828#comment8. The only deadlocks we've actually seen are on
	// imports_unique, because they can occur when processing two versions of
	// the same module, which happens regularly. But if we were ever to process
	// the same module and version twice, we could see deadlocks in the other
	// bulk inserts.
	sort.Slice(m.Packages, func(i, j int) bool {
		return m.Packages[i].Path < m.Packages[j].Path
	})
	sort.Slice(m.Licenses, func(i, j int) bool {
		return m.Licenses[i].FilePath < m.Licenses[j].FilePath
	})
	for _, p := range m.Packages {
		sort.Strings(p.Imports)
	}

	return db.db.Transact(func(tx *database.DB) error {
		// If the version exists, delete it to force an overwrite. This allows us
		// to selectively repopulate data after a code change.
		if err := db.DeleteModule(ctx, tx, m.ModulePath, m.Version); err != nil {
			return fmt.Errorf("error deleting existing versions: %v", err)
		}
		moduleID, err := insertModule(ctx, tx, m)
		if err != nil {
			return err
		}
		if err := insertLicenses(ctx, tx, m, moduleID); err != nil {
			return err
		}
		if err := insertPackages(ctx, tx, m); err != nil {
			return err
		}
		return nil
	})
}

func insertModule(ctx context.Context, db *database.DB, m *internal.Module) (_ int, err error) {
	defer derrors.Wrap(&err, "insertModule(ctx, %q, %q)", m.ModulePath, m.Version)
	sourceInfoJSON, err := json.Marshal(m.SourceInfo)
	if err != nil {
		return 0, err
	}
	var moduleID int
	err = db.QueryRow(ctx,
		`INSERT INTO modules(
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
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, $11)
		ON CONFLICT
			(module_path, version)
		DO UPDATE SET
			readme_file_path=excluded.readme_file_path,
			readme_contents=excluded.readme_contents,
			source_info=excluded.source_info,
			redistributable=excluded.redistributable
		RETURNING id`,
		m.ModulePath,
		m.Version,
		m.CommitTime,
		m.ReadmeFilePath,
		makeValidUnicode(m.ReadmeContents),
		version.ForSorting(m.Version),
		m.VersionType,
		m.SeriesPath(),
		sourceInfoJSON,
		m.IsRedistributable,
		m.HasGoMod,
	).Scan(&moduleID)
	if err != nil {
		return 0, err
	}
	return moduleID, nil
}

func insertLicenses(ctx context.Context, db *database.DB, m *internal.Module, moduleID int) (err error) {
	defer derrors.Wrap(&err, "insertLicenses(ctx, %q, %q)", m.ModulePath, m.Version)
	var licenseValues []interface{}
	for _, l := range m.Licenses {
		covJSON, err := json.Marshal(l.Coverage)
		if err != nil {
			return fmt.Errorf("marshalling %+v: %v", l.Coverage, err)
		}
		licenseValues = append(licenseValues, m.ModulePath, m.Version,
			l.FilePath, makeValidUnicode(string(l.Contents)), pq.Array(l.Types), covJSON, moduleID)
	}
	if len(licenseValues) > 0 {
		licenseCols := []string{
			"module_path",
			"version",
			"file_path",
			"contents",
			"types",
			"coverage",
			"module_id",
		}
		return db.BulkInsert(ctx, "licenses", licenseCols, licenseValues,
			database.OnConflictDoNothing)
	}
	return nil
}

func insertPackages(ctx context.Context, db *database.DB, m *internal.Module) (err error) {
	defer derrors.Wrap(&err, "insertPackages(ctx, %q, %q)", m.ModulePath, m.Version)
	// We only insert into imports_unique if this is the latest
	// version of the module.
	isLatest, err := isLatestVersion(ctx, db, m.ModulePath, m.Version)
	if err != nil {
		return err
	}
	if isLatest {
		// Remove the previous rows for this module. We'll replace them with
		// new ones below.
		if _, err := db.Exec(ctx,
			`DELETE FROM imports_unique WHERE from_module_path = $1`,
			m.ModulePath); err != nil {
			return err
		}
	}

	var pkgValues, importValues, importUniqueValues []interface{}
	for _, p := range m.Packages {
		if p.DocumentationHTML == internal.StringFieldMissing {
			return errors.New("saveModule: package missing DocumentationHTML")
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
			m.Version,
			m.ModulePath,
			p.V1Path,
			p.IsRedistributable,
			p.DocumentationHTML,
			pq.Array(licenseTypes),
			pq.Array(licensePaths),
			p.GOOS,
			p.GOARCH,
			m.CommitTime,
		)
		for _, i := range p.Imports {
			importValues = append(importValues, p.Path, m.ModulePath, m.Version, i)
			if isLatest {
				importUniqueValues = append(importUniqueValues, p.Path, m.ModulePath, i)
			}
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
		if err := db.BulkInsert(ctx, "packages", pkgCols, pkgValues, database.OnConflictDoNothing); err != nil {
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
		if err := db.BulkInsert(ctx, "imports", importCols, importValues, database.OnConflictDoNothing); err != nil {
			return err
		}
		if len(importUniqueValues) > 0 {
			importUniqueCols := []string{
				"from_path",
				"from_module_path",
				"to_path",
			}
			if err := db.BulkInsert(ctx, "imports_unique", importUniqueCols, importUniqueValues, database.OnConflictDoNothing); err != nil {
				return err
			}
		}
	}
	return nil
}

// isLatestVersion reports whether version is the latest version of the module.
func isLatestVersion(ctx context.Context, db *database.DB, modulePath, version string) (_ bool, err error) {
	defer derrors.Wrap(&err, "isLatestVersion(ctx, tx, %q)", modulePath)

	row := db.QueryRow(ctx, `
		SELECT version FROM modules WHERE module_path = $1
		ORDER BY version_type = 'release' DESC, sort_version DESC
		LIMIT 1`,
		modulePath)
	var v string
	if err := row.Scan(&v); err != nil {
		if err == sql.ErrNoRows {
			return true, nil // It's the only version, so it's also the latest.
		}
		return false, err
	}
	return version == v, nil
}

// validateModule checks that fields needed to insert a module into the
// database are present. Otherwise, it returns an error listing the reasons the
// module cannot be inserted.
func validateModule(m *internal.Module) error {
	if m == nil {
		return fmt.Errorf("nil module")
	}

	var errReasons []string
	if m.Version == "" {
		errReasons = append(errReasons, "no specified version")
	}
	if m.ModulePath == "" {
		errReasons = append(errReasons, "no module path")
	}
	if m.ModulePath != stdlib.ModulePath {
		if err := module.CheckPath(m.ModulePath); err != nil {
			errReasons = append(errReasons, fmt.Sprintf("invalid module path (%s)", err))
		}
		if !semver.IsValid(m.Version) {
			errReasons = append(errReasons, "invalid version")
		}
	}
	if len(m.Packages) == 0 {
		errReasons = append(errReasons, "module does not have any packages")
	}
	if m.CommitTime.IsZero() {
		errReasons = append(errReasons, "empty commit time")
	}
	if len(errReasons) == 0 {
		return nil
	}
	return fmt.Errorf("cannot insert module %q: %s", m.Version, strings.Join(errReasons, ", "))
}

// removeNonDistributableData removes any information from the version payload,
// after checking licenses.
func removeNonDistributableData(m *internal.Module) {
	for _, p := range m.Packages {
		if !p.IsRedistributable {
			// Prune derived information that can't be stored.
			p.Synopsis = ""
			p.DocumentationHTML = ""
		}
	}
	if !m.IsRedistributable {
		m.ReadmeFilePath = ""
		m.ReadmeContents = ""
	}
}

// DeleteModule deletes a Version from the database.
func (db *DB) DeleteModule(ctx context.Context, ddb *database.DB, modulePath, version string) (err error) {
	defer derrors.Wrap(&err, "DB.DeleteModule(ctx, ddb, %q, %q)", modulePath, version)

	if ddb == nil {
		ddb = db.db
	}
	// We only need to delete from the modules table. Thanks to ON DELETE
	// CASCADE constraints, that will trigger deletions from all other tables.
	const stmt = `DELETE FROM modules WHERE module_path=$1 AND version=$2`
	_, err = ddb.Exec(ctx, stmt, modulePath, version)
	return err
}

// makeValidUnicode removes null runes from a string that will be saved in a
// column of type TEXT, because pq doesn't like them. It also replaces non-unicode
// characters with the Unicode replacement character, which is the behavior of
// for ... range on strings.
func makeValidUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r != 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
