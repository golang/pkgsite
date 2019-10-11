// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/xerrors"
)

// InsertVersion inserts a version into the database using
// db.saveVersion, along with a search document corresponding to each of its
// packages.
func (db *DB) InsertVersion(ctx context.Context, version *internal.Version) (err error) {
	defer func() {
		if version == nil {
			derrors.Wrap(&err, "DB.InsertVersion(ctx, nil)")
		} else {
			derrors.Wrap(&err, "DB.InsertVersion(ctx, Version(%q, %q))", version.ModulePath, version.Version)
		}
	}()

	if err := validateVersion(version); err != nil {
		return xerrors.Errorf("validateVersion: %v: %w", err, derrors.InvalidArgument)
	}
	removeNonDistributableData(version)

	if err := db.saveVersion(ctx, version); err != nil {
		return err
	}
	if err := db.legacyInsertDocuments(ctx, version); err != nil {
		return err
	}
	for _, pkg := range version.Packages {
		if err := db.UpsertSearchDocument(ctx, pkg.Path); err != nil && !xerrors.Is(err, derrors.InvalidArgument) {
			return err
		}
	}
	return nil
}

// saveVersion inserts a Version into the database along with its packages,
// imports, and licenses.  If any of these rows already exist, the version and
// corresponding will be deleted and reinserted.
//
// The version string is also parsed into major, minor, patch
// and prerelease used solely for sorting database queries by semantic version.
// The prerelease column will pad any number fields with zeroes on the left so
// all number fields in the prerelease column have 20 characters. If the
// version is malformed then insertion will fail.
//
// A derrors.InvalidArgument error will be returned if the given version and
// licenses are invalid.
func (db *DB) saveVersion(ctx context.Context, version *internal.Version) error {
	// Sort to ensure proper lock ordering, avoiding deadlocks. See
	// b/141164828#comment8. The only deadlocks we've actually seen are on
	// imports_unique, because they can occur when processing two versions of
	// the same module, which happens regularly. But if we were ever to process
	// the same module and version twice, we could see deadlocks in the other
	// bulk inserts.
	sort.Slice(version.Packages, func(i, j int) bool {
		return version.Packages[i].Path < version.Packages[j].Path
	})
	sort.Slice(version.Licenses, func(i, j int) bool {
		return version.Licenses[i].FilePath < version.Licenses[j].FilePath
	})
	for _, p := range version.Packages {
		sort.Strings(p.Imports)
	}

	err := db.Transact(func(tx *sql.Tx) error {
		majorint, minorint, patchint, prerelease, err := extractSemverParts(version.Version)
		if err != nil {
			return fmt.Errorf("extractSemverParts(%q): %v", version.Version, err)
		}

		// If the version exists, delete it to force an overwrite. This allows us
		// to selectively repopulate data after a code change.
		if err := db.DeleteVersion(ctx, tx, version.ModulePath, version.Version); err != nil {
			return fmt.Errorf("error deleting existing versions: %v", err)
		}
		if _, err := execTx(ctx, tx,
			`INSERT INTO versions(
				module_path,
				version,
				commit_time,
				readme_file_path,
				readme_contents,
				major,
				minor,
				patch,
				prerelease,
				version_type,
				vcs_type,
				repository_url,
				homepage_url,
				series_path)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
			version.ModulePath,
			version.Version,
			version.CommitTime,
			version.ReadmeFilePath,
			version.ReadmeContents,
			majorint,
			minorint,
			patchint,
			prerelease,
			version.VersionType,
			version.VCSType,
			version.RepositoryURL,
			version.HomepageURL,
			version.SeriesPath(),
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		var licenseValues []interface{}
		for _, l := range version.Licenses {
			licenseValues = append(licenseValues, version.ModulePath, version.Version, l.FilePath, l.Contents, pq.Array(l.Types))
		}
		if len(licenseValues) > 0 {
			licenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"contents",
				"types",
			}
			if err := bulkInsert(ctx, tx, "licenses", licenseCols, licenseValues, onConflictDoNothing); err != nil {
				return err
			}
		}

		var pkgValues, importValues, importUniqueValues []interface{}
		for _, p := range version.Packages {
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
				version.Version,
				version.ModulePath,
				p.V1Path,
				p.IsRedistributable(),
				p.DocumentationHTML,
				pq.Array(licenseTypes),
				pq.Array(licensePaths),
				p.GOOS,
				p.GOARCH,
				version.CommitTime,
			)
			for _, i := range p.Imports {
				importValues = append(importValues, p.Path, version.ModulePath, version.Version, i)
				importUniqueValues = append(importUniqueValues, p.Path, version.ModulePath, i)
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
			if err := bulkInsert(ctx, tx, "packages", pkgCols, pkgValues, onConflictDoNothing); err != nil {
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
			if err := bulkInsert(ctx, tx, "imports", importCols, importValues, onConflictDoNothing); err != nil {
				return err
			}

			importUniqueCols := []string{
				"from_path",
				"from_module_path",
				"to_path",
			}
			if err := bulkInsert(ctx, tx, "imports_unique", importUniqueCols, importUniqueValues, onConflictDoNothing); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return xerrors.Errorf("DB.saveVersion(ctx, Version(%q, %q)): %w", version.ModulePath, version.Version, err)
	}
	return nil
}

// validateVersion checks that fields needed to insert a version into the
// database are present. Otherwise, it returns an error listing the reasons the
// version cannot be inserted.
func validateVersion(version *internal.Version) error {
	if version == nil {
		return fmt.Errorf("nil version")
	}

	var errReasons []string
	if !utf8.Valid(version.ReadmeContents) {
		errReasons = append(errReasons, fmt.Sprintf("readme %q is not valid utf8", version.ReadmeFilePath))
	}
	for _, l := range version.Licenses {
		if !utf8.Valid(l.Contents) {
			errReasons = append(errReasons, fmt.Sprintf("license %q contains invalid unicode", l.FilePath))
		}
	}
	if version.Version == "" {
		errReasons = append(errReasons, "no specified version")
	}
	if version.ModulePath == "" {
		errReasons = append(errReasons, "no module path")
	}
	if version.ModulePath != stdlib.ModulePath {
		if err := module.CheckPath(version.ModulePath); err != nil {
			errReasons = append(errReasons, "invalid module path")
		}
		if !semver.IsValid(version.Version) {
			errReasons = append(errReasons, "invalid version")
		}
	}
	if len(version.Packages) == 0 {
		errReasons = append(errReasons, "module does not have any packages")
	}
	if version.CommitTime.IsZero() {
		errReasons = append(errReasons, "empty commit time")
	}
	if len(errReasons) == 0 {
		return nil
	}
	return fmt.Errorf("cannot insert version %q: %s", version.Version, strings.Join(errReasons, ", "))
}

// removeNonDistributableData removes any information from the version payload,
// after checking licenses.
func removeNonDistributableData(v *internal.Version) {
	hasRedistributablePackage := false
	for _, p := range v.Packages {
		if p.IsRedistributable() {
			hasRedistributablePackage = true
		} else {
			// Not redistributable, so prune derived information
			// that can't be stored.
			p.Synopsis = ""
			p.DocumentationHTML = nil
		}
	}

	// If no packages are redistributable, we have no need for the readme
	// contents, so drop them. Note that if a single package is redistributable,
	// it must be true by definition that the module itself it redistributable,
	// so capturing the README contents is OK.
	if !hasRedistributablePackage {
		v.ReadmeFilePath = ""
		v.ReadmeContents = nil
	}
}

// extractSemverParts extracts the major, minor, patch and prerelease from
// version to be used for sorting versions in the database. The prerelease
// string is padded with zeroes so that the resulting field is 20 characters
// and returns the string "~" if it is empty.
func extractSemverParts(version string) (majorint, minorint, patchint int, prerelease string, err error) {
	majorint, err = major(version)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("major(%q): %v", version, err)
	}

	minorint, err = minor(version)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("minor(%q): %v", version, err)
	}

	patchint, err = patch(version)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("patch(%q): %v", version, err)
	}

	prerelease, err = padPrerelease(version)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("padPrerelease(%q): %v", version, err)
	}
	return majorint, minorint, patchint, prerelease, nil
}

// major returns the major version integer value of the semantic version
// v.  For example, major("v2.1.0") == 2.
func major(v string) (int, error) {
	m := strings.TrimPrefix(semver.Major(v), "v")
	major, err := strconv.Atoi(m)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", m, err)
	}
	return major, nil
}

// minor returns the minor version integer value of the semantic version For
// example, minor("v2.1.0") == 1.
func minor(v string) (int, error) {
	m := strings.TrimPrefix(semver.MajorMinor(v), fmt.Sprintf("%s.", semver.Major(v)))
	minor, err := strconv.Atoi(m)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", m, err)
	}
	return minor, nil
}

// patch returns the patch version integer value of the semantic version For
// example, patch("v2.1.0+incompatible") == 0.
func patch(v string) (int, error) {
	s := strings.TrimPrefix(semver.Canonical(v), fmt.Sprintf("%s.", semver.MajorMinor(v)))
	p := strings.TrimSuffix(s, semver.Prerelease(v))
	patch, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", p, err)
	}
	return patch, nil
}

// padPrerelease returns '~' if the given string is empty
// and otherwise pads all number fields with zeroes so that
// the resulting field is 20 characters and returns that
// string without the '-' prefix. The '~' is returned so that
// full releases will take greatest precedence when sorting
// in ASCII sort order. The given string may only contain
// lowercase letters, numbers, periods, hyphens or nothing.
func padPrerelease(v string) (string, error) {
	p := semver.Prerelease(v)
	if p == "" {
		return "~", nil
	}

	pre := strings.Split(strings.TrimPrefix(p, "-"), ".")
	var err error
	for i, segment := range pre {
		if isNum(segment) {
			pre[i], err = prefixZeroes(segment)
			if err != nil {
				return "", fmt.Errorf("padRelease(%v): number field %v is longer than 20 characters", p, segment)
			}
		}
	}
	return strings.Join(pre, "."), nil
}

// prefixZeroes returns a string that is padded with zeroes on the
// left until the string is exactly 20 characters long. If the string
// is already 20 or more characters it is returned unchanged. 20
// characters being the length because the length of a date in the form
// yyyymmddhhmmss has 14 characters and that is longest number that
// is expected to be found in a prerelease number field.
func prefixZeroes(s string) (string, error) {
	if len(s) > 20 {
		return "", fmt.Errorf("prefixZeroes(%v): input string is more than 20 characters", s)
	}

	if len(s) == 20 {
		return s, nil
	}

	var padded []string

	for i := 0; i < 20-len(s); i++ {
		padded = append(padded, "0")
	}

	return strings.Join(append(padded, s), ""), nil
}

// isNum returns true if every character in a string is a number
// and returns false otherwise.
func isNum(v string) bool {
	i := 0
	for i < len(v) && '0' <= v[i] && v[i] <= '9' {
		i++
	}
	return len(v) > 0 && i == len(v)
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
		_, err = db.exec(ctx, stmt, modulePath, version)
	} else {
		_, err = execTx(ctx, tx, stmt, modulePath, version)
	}
	return err
}
