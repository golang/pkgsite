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
	"hash/fnv"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"
	"go.opencensus.io/trace"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// InsertModule inserts a version into the database using db.saveVersion, along
// with a search document corresponding to each of its packages.
// It returns whether the version inserted was the latest for the given module path.
func (db *DB) InsertModule(ctx context.Context, m *internal.Module, lmv *internal.LatestModuleVersions) (isLatest bool, err error) {
	defer func() {
		if m == nil {
			derrors.WrapStack(&err, "DB.InsertModule(ctx, nil)")
			return
		}
		derrors.WrapStack(&err, "DB.InsertModule(ctx, Module(%q, %q))", m.ModulePath, m.Version)
	}()

	if err := validateModule(m); err != nil {
		return false, err
	}
	// The proxy accepts modules with zero commit times, but they are bad.
	if m.CommitTime.IsZero() {
		return false, fmt.Errorf("empty commit time: %w", derrors.BadModule)
	}
	// Compare existing data from the database, and the module to be
	// inserted. Rows that currently exist should not be missing from the
	// new module. We want to be sure that we will overwrite every row that
	// pertains to the module.
	if err := db.comparePaths(ctx, m); err != nil {
		return false, err
	}
	if !db.bypassLicenseCheck {
		// If we are not bypassing license checking, remove data for non-redistributable modules.
		m.RemoveNonRedistributableData()
	}
	return db.saveModule(ctx, m, lmv)
}

// saveModule inserts a Module into the database along with its packages,
// imports, and licenses.  If any of these rows already exist, the module and
// corresponding will be deleted and reinserted.
// If the module is malformed then insertion will fail.
//
// saveModule reports whether the version inserted is the latest known version
// for the module path (that is, the latest minor version of the module)
//
// A derrors.InvalidArgument error will be returned if the given module and
// licenses are invalid.
func (db *DB) saveModule(ctx context.Context, m *internal.Module, lmv *internal.LatestModuleVersions) (isLatest bool, err error) {
	defer derrors.WrapStack(&err, "saveModule(ctx, tx, Module(%q, %q))", m.ModulePath, m.Version)
	ctx, span := trace.StartSpan(ctx, "saveModule")
	defer span.End()

	// Insert paths first in a separate transaction, because we've seen various
	// problems like deadlock when we do it as part of the main transaction
	// below. Without RepeatableRead, insertPaths can fail to return some paths.
	// For details, see the commit message for https://golang.org/cl/290269.
	var pathToID map[string]int
	err = db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		var err error
		pathToID, err = insertPaths(ctx, tx, m)
		return err
	})
	if err != nil {
		return false, err
	}

	err = db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		moduleID, err := insertModule(ctx, tx, m)
		if err != nil {
			return err
		}
		// Compare existing data from the database, and the module to be
		// inserted. Rows that currently exist should not be missing from the
		// new module. We want to be sure that we will overwrite every row that
		// pertains to the module.
		if err := db.compareLicenses(ctx, moduleID, m.Licenses); err != nil {
			return err
		}
		if err := insertLicenses(ctx, tx, m, moduleID); err != nil {
			return err
		}
		pathToUnitID, pathToDocs, err := db.insertUnits(ctx, tx, m, moduleID, pathToID)
		if err != nil {
			return err
		}

		// Obtain a transaction-scoped exclusive advisory lock on the module
		// path. The transaction that holds the lock is the only one that can
		// execute the subsequent code on any module with the given path. That
		// means that conflicts from two transactions both believing they are
		// working on the latest version of a given module cannot happen.
		// The lock is released automatically at the end of the transaction.
		if err := lock(ctx, tx, m.ModulePath); err != nil {
			return err
		}

		// We only insert into imports_unique and search_documents if this is
		// the latest version of the module.
		// By the time this function is called, we've already inserted into the modules table.
		// So the query in getLatestGoodVersion will include this version.
		latest, err := getLatestGoodVersion(ctx, tx, m.ModulePath, lmv)
		if err != nil {
			return err
		}
		// Update the DB with the latest version, even if we are not the latest.
		// (Perhaps we just learned of a retraction that affects the good latest
		// version.)
		if err := updateLatestGoodVersion(ctx, tx, m.ModulePath, latest); err != nil {
			return err
		}
		isLatest = m.Version == latest
		if err := insertSymbols(ctx, tx, m.ModulePath, m.Version, isLatest, pathToID, pathToUnitID, pathToDocs); err != nil {
			return err
		}
		if !isLatest {
			return nil
		}

		// Here, this module is the latest good version.

		if err := insertImportsUnique(ctx, tx, m); err != nil {
			return err
		}

		if err := deleteOtherModulePackagesFromSearchDocuments(ctx, tx, m); err != nil {
			return err
		}

		// If the most recent version of this module has an alternative module
		// path, then do not insert its packages into search_documents (and
		// delete whatever is there). This happens when a module that initially
		// does not have a go.mod file is forked or fetched via some
		// non-canonical path (such as an alternative capitalization), and then
		// in a later version acquires a go.mod file.
		//
		// To take an actual example: github.com/sirupsen/logrus@v1.1.0 has a go.mod
		// file that establishes that path as canonical. But v1.0.6 does not have a
		// go.mod file. So the miscapitalized path github.com/Sirupsen/logrus at
		// v1.1.0 is marked as an alternative path (code 491) by
		// internal/fetch.FetchModule and is not inserted into the DB, but at
		// v1.0.6 it is considered valid, and we end up here. We still insert
		// github.com/Sirupsen/logrus@v1.0.6 in the modules table and friends so
		// that users who import it can find information about it, but we don't want
		// it showing up in search results.
		//
		// Note that we end up here only if we first saw the alternative version
		// (github.com/Sirupsen/logrus@v1.1.0 in the example) and then see the valid
		// one. The "if code == 491" section of internal/worker.fetchAndUpdateState
		// handles the case where we fetch the versions in the other order.
		alt, err := isAlternativeModulePath(ctx, tx, m.ModulePath)
		if err != nil {
			return err
		}
		if alt {
			log.Infof(ctx, "%s@%s: not inserting into search documents", m.ModulePath, m.Version)
			return nil
		}
		// Insert the module's packages into search_documents.
		if err := upsertSearchDocuments(ctx, tx, m); err != nil {
			return err
		}
		return upsertSymbolSearchDocuments(ctx, tx, m.ModulePath, m.Version)
	})
	if err != nil {
		return false, err
	}
	return isLatest, nil
}

// isAlternativeModulePath reports whether the module path is "alternative,"
// that is, it disagrees with the module path in the go.mod file. This can
// happen when someone forks a repo and does not change the go.mod file, or when
// the path used to get a module is a case variant of the correct one (e.g.
// github.com/Sirupsen/logrus vs. github.com/sirupsen/logrus).
func isAlternativeModulePath(ctx context.Context, db *database.DB, modulePath string) (_ bool, err error) {
	defer derrors.WrapStack(&err, "isAlternativeModulePath(%q)", modulePath)

	// See if the cooked latest version has a status of 491 (AlternativeModule).
	var status int
	switch err := db.QueryRow(ctx, `
		SELECT s.status
		FROM paths p, latest_module_versions l, module_version_states s
		WHERE p.id = l.module_path_id
		AND p.path = s.module_path
		AND l.cooked_version = s.version
		AND s.module_path = $1
	`, modulePath).Scan(&status); err {
	case sql.ErrNoRows:
		// Not enough information; assume false so we don't omit a valid module
		// from search.
		return false, nil
	case nil:
		return status == derrors.ToStatus(derrors.AlternativeModule), nil
	default:
		return false, err
	}
}

func insertModule(ctx context.Context, db *database.DB, m *internal.Module) (_ int, err error) {
	ctx, span := trace.StartSpan(ctx, "insertModule")
	defer span.End()
	defer derrors.WrapStack(&err, "insertModule(ctx, %q, %q)", m.ModulePath, m.Version)
	sourceInfoJSON, err := json.Marshal(m.SourceInfo)
	if err != nil {
		return 0, err
	}
	versionType, err := version.ParseType(m.Version)
	if err != nil {
		return 0, err
	}
	var moduleID int
	err = db.QueryRow(ctx,
		`INSERT INTO modules(
			module_path,
			version,
			commit_time,
			sort_version,
			version_type,
			series_path,
			source_info,
			redistributable,
			has_go_mod,
			incompatible)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT
			(module_path, version)
		DO UPDATE SET
			source_info=excluded.source_info,
			redistributable=excluded.redistributable
		RETURNING id`,
		m.ModulePath,
		m.Version,
		m.CommitTime,
		version.ForSorting(m.Version),
		versionType,
		m.SeriesPath(),
		sourceInfoJSON,
		m.IsRedistributable,
		m.HasGoMod,
		version.IsIncompatible(m.Version),
	).Scan(&moduleID)
	if err != nil {
		return 0, err
	}
	return moduleID, nil
}

func insertLicenses(ctx context.Context, db *database.DB, m *internal.Module, moduleID int) (err error) {
	ctx, span := trace.StartSpan(ctx, "insertLicenses")
	defer span.End()
	defer derrors.WrapStack(&err, "insertLicenses(ctx, %q, %q)", m.ModulePath, m.Version)
	var licenseValues []interface{}
	for _, l := range m.Licenses {
		var covJSON []byte
		if l.Coverage.Percent == 0 && l.Coverage.Match == nil {
			covJSON, err = json.Marshal(l.OldCoverage)
			if err != nil {
				return fmt.Errorf("marshalling %+v: %v", l.OldCoverage, err)
			}
		} else {
			covJSON, err = json.Marshal(l.Coverage)
			if err != nil {
				return fmt.Errorf("marshalling %+v: %v", l.Coverage, err)
			}
		}
		licenseValues = append(licenseValues, l.FilePath,
			makeValidUnicode(string(l.Contents)), pq.Array(l.Types), covJSON,
			moduleID)
	}
	if len(licenseValues) > 0 {
		licenseCols := []string{
			"file_path",
			"contents",
			"types",
			"coverage",
			"module_id",
		}
		return db.BulkUpsert(ctx, "licenses", licenseCols, licenseValues,
			[]string{"module_id", "file_path"})
	}
	return nil
}

// insertImportsUnique inserts and removes rows from the imports_unique table. It should only
// be called if the given module's version is the latest.
func insertImportsUnique(ctx context.Context, tx *database.DB, m *internal.Module) (err error) {
	ctx, span := trace.StartSpan(ctx, "insertImportsUnique")
	defer span.End()
	defer derrors.WrapStack(&err, "insertImportsUnique(%q, %q)", m.ModulePath, m.Version)

	// Remove the previous rows for this module. We'll replace them with
	// new ones below.
	if err := deleteModuleFromImportsUnique(ctx, tx, m.ModulePath); err != nil {
		return err
	}

	var values []interface{}
	for _, u := range m.Units {
		for _, i := range u.Imports {
			values = append(values, u.Path, m.ModulePath, i)
		}
	}
	if len(values) == 0 {
		return nil
	}
	cols := []string{"from_path", "from_module_path", "to_path"}
	return tx.BulkUpsert(ctx, "imports_unique", cols, values, cols)
}

// insertUnits inserts the units for a module into the units table.
// It must be called inside a transaction.
//
// It can be assume that at least one unit is a package, and there are one or
// more units in the module.
func (pdb *DB) insertUnits(ctx context.Context, tx *database.DB,
	m *internal.Module, moduleID int, pathToID map[string]int) (
	pathToUnitID map[string]int, pathToPkgDocs map[string][]*internal.Documentation, err error) {
	defer derrors.WrapStack(&err, "insertUnits(ctx, tx, %q, %q)", m.ModulePath, m.Version)
	ctx, span := trace.StartSpan(ctx, "insertUnits")
	defer span.End()

	// Sort to ensure proper lock ordering, avoiding deadlocks. We have seen
	// deadlocks on package_imports and documentation. They can occur when
	// processing two versions of the same module, which happens regularly.
	sort.Slice(m.Units, func(i, j int) bool {
		return m.Units[i].Path < m.Units[j].Path
	})
	for _, u := range m.Units {
		sort.Strings(u.Imports)
	}
	var (
		paths         []string
		unitValues    []interface{}
		pathToReadme  = map[string]*internal.Readme{}
		pathToImports = map[string][]string{}
		pathIDToPath  = map[int]string{}
		pathToAllDocs = map[string][]*internal.Documentation{}
	)
	pathToPkgDocs = map[string][]*internal.Documentation{}
	for _, u := range m.Units {
		var licenseTypes, licensePaths []string
		for _, l := range u.Licenses {
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
		v1path := internal.V1Path(u.Path, m.ModulePath)
		pathID, ok := pathToID[u.Path]
		if !ok {
			return nil, nil, fmt.Errorf("no entry in paths table for %q; should be impossible", u.Path)
		}
		pathIDToPath[pathID] = u.Path
		unitValues = append(unitValues,
			pathID,
			moduleID,
			pathToID[v1path],
			u.Name,
			pq.Array(licenseTypes),
			pq.Array(licensePaths),
			u.IsRedistributable,
		)
		if u.Readme != nil {
			pathToReadme[u.Path] = u.Readme
		}
		for _, d := range u.Documentation {
			if d.Source == nil {
				return nil, nil, fmt.Errorf("insertUnits: unit %q missing source files for %q, %q", u.Path, d.GOOS, d.GOARCH)
			}
		}
		pathToAllDocs[u.Path] = u.Documentation
		if !u.IsCommand() {
			// We don't care about symbols for commands, since they won't
			// appear in the documentation.
			pathToPkgDocs[u.Path] = u.Documentation
		}
		if len(u.Imports) > 0 {
			pathToImports[u.Path] = u.Imports
		}
		paths = append(paths, u.Path)
	}
	pathIDToUnitID, err := insertUnits(ctx, tx, unitValues)
	if err != nil {
		return nil, nil, err
	}
	pathToUnitID = map[string]int{}
	for pid, uid := range pathIDToUnitID {
		pathToUnitID[pathIDToPath[pid]] = uid
	}
	if err := insertReadmes(ctx, tx, paths, pathToUnitID, pathToReadme); err != nil {
		return nil, nil, err
	}
	if err := insertDocs(ctx, tx, paths, pathToUnitID, pathToAllDocs); err != nil {
		return nil, nil, err
	}
	if err := insertImports(ctx, tx, paths, pathToUnitID, pathToImports); err != nil {
		return nil, nil, err
	}
	return pathToUnitID, pathToPkgDocs, nil
}

// insertPaths inserts all paths in m that aren't already there, and returns a map from each path to its
// ID in the paths table.
// Should be run inside a transaction.
func insertPaths(ctx context.Context, tx *database.DB, m *internal.Module) (pathToID map[string]int, err error) {
	curPathsSet := map[string]bool{}
	for _, u := range m.Units {
		curPathsSet[u.Path] = true
		curPathsSet[internal.V1Path(u.Path, m.ModulePath)] = true
		curPathsSet[internal.SeriesPathForModule(m.ModulePath)] = true
	}
	return upsertPaths(ctx, tx, stringSetToSlice(curPathsSet))
}

func stringSetToSlice(m map[string]bool) []string {
	var s []string
	for e := range m {
		s = append(s, e)
	}
	return s
}

func insertUnits(ctx context.Context, db *database.DB, unitValues []interface{}) (pathIDToUnitID map[int]int, err error) {
	defer derrors.WrapAndReport(&err, "insertUnits")

	// Insert data into the units table.
	unitCols := []string{
		"path_id",
		"module_id",
		"v1path_id",
		"name",
		"license_types",
		"license_paths",
		"redistributable",
	}
	uniqueUnitCols := []string{"path_id", "module_id"}
	returningUnitCols := []string{"id", "path_id"}

	// Check to see if any rows have the same path_id and module_id.
	// For golang/go#43899.
	conflictingValues := map[[2]interface{}]bool{}
	for i := 0; i < len(unitValues); i += len(unitCols) {
		key := [2]interface{}{unitValues[i], unitValues[i+1]}
		if conflictingValues[key] {
			log.Errorf(ctx, "insertUnits: %v occurs twice", key)
		} else {
			conflictingValues[key] = true
		}
	}

	pathIDToUnitID = map[int]int{}
	if err := db.BulkUpsertReturning(ctx, "units", unitCols, unitValues,
		uniqueUnitCols, returningUnitCols, func(rows *sql.Rows) error {
			var pathID, unitID int
			if err := rows.Scan(&unitID, &pathID); err != nil {
				return err
			}
			pathIDToUnitID[pathID] = unitID
			return nil
		}); err != nil {
		log.Errorf(ctx, "got error doing bulk upsert to units (see below); logging path_id, module_id for golang.org/issue/43899")
		for i := 0; i < len(unitValues); i += len(unitCols) {
			log.Errorf(ctx, "%v, %v", unitValues[i], unitValues[i+1])
		}
		return nil, err
	}
	return pathIDToUnitID, nil
}

func insertDocs(ctx context.Context, db *database.DB,
	paths []string,
	pathToUnitID map[string]int,
	pathToDocs map[string][]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertDocs(%d paths)", len(paths))

	generateRows := func() chan database.RowItem {
		ch := make(chan database.RowItem)
		go func() {
			for _, path := range paths {
				unitID := pathToUnitID[path]
				for _, doc := range pathToDocs[path] {
					if doc.GOOS == "" || doc.GOARCH == "" {
						ch <- database.RowItem{Err: errors.New("empty GOOS or GOARCH")}
					}
					ch <- database.RowItem{Values: []interface{}{unitID, doc.GOOS, doc.GOARCH, doc.Synopsis, doc.Source}}
				}
			}
			close(ch)
		}()
		return ch
	}

	uniqueCols := []string{"unit_id", "goos", "goarch"}
	docCols := append(uniqueCols, "synopsis", "source")
	return db.CopyUpsert(ctx, "documentation",
		docCols, database.CopyFromChan(generateRows()), uniqueCols, "id")
}

// getDocIDsForPath returns a map of the unit path to documentation.id to
// documentation, for all of the docs in pathToDocs. This will be used to
// insert data into the documentation_symbols.documentation_id column.
func getDocIDsForPath(ctx context.Context, db *database.DB,
	pathToUnitID map[string]int,
	pathToDocs map[string][]*internal.Documentation) (_ map[string]map[int]*internal.Documentation, err error) {
	defer derrors.WrapStack(&err, "getDocIDsForPath")

	pathToDocIDToDoc := map[string]map[int]*internal.Documentation{}
	unitIDToPath := map[int]string{}
	collect := func(rows *sql.Rows) error {
		var (
			id, unitID   int
			goos, goarch string
		)
		if err := rows.Scan(&id, &unitID, &goos, &goarch); err != nil {
			return err
		}
		path := unitIDToPath[unitID]
		if _, ok := pathToDocIDToDoc[path]; !ok {
			pathToDocIDToDoc[path] = map[int]*internal.Documentation{}
		}
		for _, doc := range pathToDocs[path] {
			if doc.GOOS == goos && doc.GOARCH == goarch {
				pathToDocIDToDoc[path][id] = doc
			}
		}
		return nil
	}

	var unitIDs []int
	for path := range pathToDocs {
		unitIDToPath[pathToUnitID[path]] = path
		unitIDs = append(unitIDs, pathToUnitID[path])
	}

	q := `SELECT id, unit_id, goos, goarch FROM documentation WHERE unit_id = ANY($1)`
	if err := db.RunQuery(ctx, q, collect, pq.Array(unitIDs)); err != nil {
		return nil, err
	}
	return pathToDocIDToDoc, nil
}

func insertImports(ctx context.Context, tx *database.DB,
	paths []string,
	pathToUnitID map[string]int,
	pathToImports map[string][]string) (err error) {
	defer derrors.WrapStack(&err, "insertImports")

	importPathSet := map[string]bool{}
	for _, pkgPath := range paths {
		for _, imp := range pathToImports[pkgPath] {
			importPathSet[imp] = true
		}
	}
	pathToID, err := upsertPaths(ctx, tx, stringSetToSlice(importPathSet))
	if err != nil {
		return err
	}

	var importValues []interface{}
	for _, pkgPath := range paths {
		imports, ok := pathToImports[pkgPath]
		if !ok {
			continue
		}
		unitID := pathToUnitID[pkgPath]
		for _, toPath := range imports {
			pathID, ok := pathToID[toPath]
			if !ok {
				return fmt.Errorf("no ID for path %q; shouldn't happen", toPath)
			}
			importValues = append(importValues, unitID, pathID)
		}
	}
	importCols := []string{"unit_id", "to_path_id"}
	return tx.BulkUpsert(ctx, "imports", importCols, importValues, importCols)
}

func insertReadmes(ctx context.Context, db *database.DB,
	paths []string,
	pathToUnitID map[string]int,
	pathToReadme map[string]*internal.Readme) (err error) {
	defer derrors.WrapStack(&err, "insertReadmes")

	var readmeValues []interface{}
	for _, path := range paths {
		readme, ok := pathToReadme[path]
		if !ok {
			continue
		}

		// Do not add a readme with empty or zero contents.
		readmeContents := makeValidUnicode(readme.Contents)
		if len(readmeContents) == 0 {
			continue
		}

		unitID := pathToUnitID[path]
		readmeValues = append(readmeValues, unitID, readme.Filepath, readmeContents)
	}
	readmeCols := []string{"unit_id", "file_path", "contents"}
	return db.BulkUpsert(ctx, "readmes", readmeCols, readmeValues, []string{"unit_id"})
}

// ReInsertLatestVersion checks that the latest good version matches the version
// in search_documents. If it doesn't, it inserts the latest good version into
// search_documents and imports_unique.
func (db *DB) ReInsertLatestVersion(ctx context.Context, modulePath string) (err error) {
	defer derrors.WrapStack(&err, "ReInsertLatestVersion(%q)", modulePath)

	return db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		// Hold the lock on the module path throughout.
		if err := lock(ctx, tx, modulePath); err != nil {
			return err
		}

		lmv, _, err := getLatestModuleVersions(ctx, tx, modulePath)
		if err != nil {
			return err
		}
		if lmv == nil {
			log.Debugf(ctx, "ReInsertLatestVersion(%q): no latest-version info", modulePath)
			return nil
		}
		if lmv.GoodVersion == "" {
			// A missing GoodVersion means that there are no good versions
			// remaining, and we should remove the current module from
			// search_documents.
			if err := deleteModuleOrPackagesInModuleFromSearchDocuments(ctx, tx, modulePath, nil); err != nil {
				return err
			}
			if err := deleteModuleFromImportsUnique(ctx, tx, modulePath); err != nil {
				return err
			}
			log.Debugf(ctx, "ReInsertLatestVersion(%q): no good version; removed from search_documents and imports_unique", modulePath)
		}
		// Is the latest good version in search_documents?
		var x int
		switch err := tx.QueryRow(ctx, `
			SELECT 1
			FROM search_documents
			WHERE module_path = $1
			AND version = $2
		`, modulePath, lmv.GoodVersion).Scan(&x); err {
		case sql.ErrNoRows:
			break
		case nil:
			log.Debugf(ctx, "ReInsertLatestVersion(%q): good version %s found in search_documents; doing nothing",
				modulePath, lmv.GoodVersion)
			return nil
		default:
			return err
		}

		// The latest good version is not in search_documents. Is this an
		// alternative module path?
		alt, err := isAlternativeModulePath(ctx, tx, modulePath)
		if err != nil {
			return err
		}
		if alt {
			log.Debugf(ctx, "ReInsertLatestVersion(%q): alternative module path; doing nothing", modulePath)
			return nil
		}

		// Not an alternative module path. Read the module information at the
		// latest good version.
		pkgMetas, err := getPackagesInUnit(ctx, tx, modulePath, modulePath, lmv.GoodVersion, -1, db.bypassLicenseCheck)
		if err != nil {
			return err
		}
		// We only need the readme for the module.
		readme, err := getModuleReadme(ctx, tx, modulePath, lmv.GoodVersion)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return err
		}

		// Insert into search_documents.
		for _, pkg := range pkgMetas {
			if isInternalPackage(pkg.Path) {
				continue
			}
			args := UpsertSearchDocumentArgs{
				PackagePath: pkg.Path,
				ModulePath:  modulePath,
				Version:     lmv.GoodVersion,
				Synopsis:    pkg.Synopsis,
			}
			if pkg.Path == modulePath && readme != nil {
				args.ReadmeFilePath = readme.Filepath
				args.ReadmeContents = readme.Contents
			}
			if err := UpsertSearchDocument(ctx, tx, args); err != nil {
				return err
			}
		}

		// Remove old rows from imports_unique.
		if err := deleteModuleFromImportsUnique(ctx, tx, modulePath); err != nil {
			return err
		}

		// Insert this version's imports into imports_unique.
		if _, err := tx.Exec(ctx, `
				INSERT INTO imports_unique (from_path, from_module_path, to_path)
				SELECT p1.path, m.module_path, p2.path
				FROM units u
				INNER JOIN imports i ON u.id = i.unit_id
				INNER JOIN paths p1 ON p1.id = u.path_id
				INNER JOIN modules m ON m.id = u.module_id
				INNER JOIN paths p2 ON p2.id = i.to_path_id
				WHERE m.module_path = $1 and m.version = $2
		`, modulePath, lmv.GoodVersion); err != nil {
			return err
		}

		log.Debugf(ctx, "ReInsertLatestVersion(%q): re-inserted at latest good version %s", modulePath, lmv.GoodVersion)
		return nil
	})
}

// lock obtains an exclusive, transaction-scoped advisory lock on modulePath.
func lock(ctx context.Context, tx *database.DB, modulePath string) (err error) {
	defer derrors.WrapStack(&err, "lock(%s)", modulePath)
	if !tx.InTransaction() {
		return errors.New("not in a transaction")
	}
	// Postgres advisory locks use a 64-bit integer key. Convert modulePath to a
	// key by hashing.
	//
	// This can result in collisions (two module paths hashing to the same key),
	// but they are unlikely and at worst will slow things down a bit.
	//
	// We use the FNV hash algorithm from the standard library. It fits into 64
	// bits unlike a crypto hash, and is stable across processes, unlike
	// hash/maphash.
	hasher := fnv.New64()
	io.WriteString(hasher, modulePath) // Writing to a hash.Hash never returns an error.
	h := int64(hasher.Sum64())
	if !database.QueryLoggingDisabled {
		log.Debugf(ctx, "locking %s (%d) ...", modulePath, h)
	}
	// See https://www.postgresql.org/docs/11/functions-admin.html#FUNCTIONS-ADVISORY-LOCKS.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, h); err != nil {
		return err
	}
	if !database.QueryLoggingDisabled {
		log.Debugf(ctx, "locking %s (%d) succeeded", modulePath, h)
	}
	return nil
}

// validateModule checks that fields needed to insert a module into the database
// are present. Otherwise, it returns an error listing the reasons the module
// cannot be inserted. Since the problems it looks for are most likely on our
// end, the underlying error it returns is always DBModuleInsertInvalid, meaning
// that this module should be reprocessed.
func validateModule(m *internal.Module) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%v: %w", err, derrors.DBModuleInsertInvalid)
			if m != nil {
				derrors.WrapStack(&err, "validateModule(%q, %q)", m.ModulePath, m.Version)
			}
		}
	}()

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
	if len(m.Packages()) == 0 {
		errReasons = append(errReasons, "module does not have any packages")
	}
	if len(errReasons) != 0 {
		return fmt.Errorf("cannot insert module %q: %s", m.Version, strings.Join(errReasons, ", "))
	}
	return nil
}

// compareLicenses compares m.Licenses with the existing licenses for
// m.ModulePath and m.Version in the database. It returns an error if there
// are licenses in the licenses table that are not present in m.Licenses.
func (db *DB) compareLicenses(ctx context.Context, moduleID int, lics []*licenses.License) (err error) {
	defer derrors.WrapStack(&err, "compareLicenses(ctx, %d)", moduleID)
	dbLicenses, err := db.getModuleLicenses(ctx, moduleID)
	if err != nil {
		return err
	}

	set := map[string]bool{}
	for _, l := range lics {
		set[l.FilePath] = true
	}
	for _, l := range dbLicenses {
		if _, ok := set[l.FilePath]; !ok {
			return fmt.Errorf("expected license %q in module: %w", l.FilePath, derrors.DBModuleInsertInvalid)
		}
	}
	return nil
}

// comparePaths compares m.Directories with the existing directories for
// m.ModulePath and m.Version in the database. It returns an error if there
// are paths in the paths table that are not present in m.Directories.
func (db *DB) comparePaths(ctx context.Context, m *internal.Module) (err error) {
	defer derrors.WrapStack(&err, "comparePaths(ctx, %q, %q)", m.ModulePath, m.Version)
	dbPaths, err := db.getPathsInModule(ctx, m.ModulePath, m.Version)
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, p := range m.Units {
		set[p.Path] = true
	}
	for _, p := range dbPaths {
		if _, ok := set[p.path]; !ok {
			return fmt.Errorf("expected unit %q in module: %w", p.path, derrors.DBModuleInsertInvalid)
		}
	}
	return nil
}

// makeValidUnicode removes null runes from a string that will be saved in a
// column of type TEXT, because pq doesn't like them. It also replaces non-unicode
// characters with the Unicode replacement character, which is the behavior of
// for ... range on strings.
func makeValidUnicode(s string) string {
	// If s is valid and has no zeroes, don't copy it.
	hasZeroes := false
	for _, r := range s {
		if r == 0 {
			hasZeroes = true
			break
		}
	}
	if !hasZeroes && utf8.ValidString(s) {
		return s
	}

	var b strings.Builder
	for _, r := range s {
		if r != 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
