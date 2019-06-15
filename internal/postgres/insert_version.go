// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// InsertVersion inserts a Version into the database along with any necessary
// series, modules and packages. If any of these rows already exist, they will
// not be updated. The version string is also parsed into major, minor, patch
// and prerelease used solely for sorting database queries by semantic version.
// The prerelease column will pad any number fields with zeroes on the left so
// all number fields in the prerelease column have 20 characters. If the
// version is malformed then insertion will fail.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine whether it was caused by an invalid version or module.
func (db *DB) InsertVersion(ctx context.Context, version *internal.Version, licenses []*license.License) error {
	if err := validateVersion(version); err != nil {
		return derrors.InvalidArgument(fmt.Sprintf("validateVersion: %v", err))
	}

	removeNonDistributableData(version)

	return db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO series(path)
			VALUES($1)
			ON CONFLICT DO NOTHING`,
			version.SeriesPath); err != nil {
			return fmt.Errorf("error inserting series: %v", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO modules(path, series_path)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.ModulePath, version.SeriesPath); err != nil {
			return fmt.Errorf("error inserting module: %v", err)
		}

		majorint, minorint, patchint, prerelease, err := extractSemverParts(version.Version)
		if err != nil {
			return fmt.Errorf("extractSemverParts(%q): %v", version.Version, err)
		}

		// If the version exists, delete it to force an overwrite. This allows us
		// to selectively repopulate data after a code change.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM versions WHERE module_path=$1 AND version=$2`,
			version.ModulePath,
			version.Version,
		); err != nil {
			return fmt.Errorf("error deleting existing versions: %v", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO versions(module_path, version, commit_time, readme_file_path, readme_contents, major, minor, patch, prerelease, version_type)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
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
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		var licenseValues []interface{}
		for _, l := range licenses {
			licenseValues = append(licenseValues, version.ModulePath, version.Version, l.FilePath, l.Contents, l.Type)
		}
		if len(licenseValues) > 0 {
			licenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"contents",
				"type",
			}
			table := "licenses"
			if err := bulkInsert(ctx, tx, table, licenseCols, licenseValues, onConflictDoNothing); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, [%d licenseValues]): %v", table, licenseCols, len(licenseValues), err)
			}
		}

		var pkgValues []interface{}
		var importValues []interface{}
		var pkgLicenseValues []interface{}
		for _, p := range version.Packages {
			pkgValues = append(pkgValues,
				p.Path,
				p.Synopsis,
				p.Name,
				version.Version,
				version.ModulePath,
				p.Suffix,
				p.IsRedistributable(),
				p.DocumentationHTML,
			)

			for _, l := range p.Licenses {
				pkgLicenseValues = append(pkgLicenseValues, version.ModulePath, version.Version, l.FilePath, p.Path)
			}

			for _, i := range p.Imports {
				importValues = append(importValues, p.Path, version.ModulePath, version.Version, i.Path, i.Name)
			}
		}
		if len(pkgValues) > 0 {
			pkgCols := []string{
				"path",
				"synopsis",
				"name",
				"version",
				"module_path",
				"suffix",
				"redistributable",
				"documentation",
			}
			table := "packages"
			if err := bulkInsert(ctx, tx, table, pkgCols, pkgValues, onConflictDoNothing); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d pkgValues): %v", table, pkgCols, len(pkgValues), err)
			}
		}
		if len(pkgLicenseValues) > 0 {
			pkgLicenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"package_path",
			}
			table := "package_licenses"
			if err := bulkInsert(ctx, tx, table, pkgLicenseCols, pkgLicenseValues, onConflictDoNothing); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d pkgLicenseValues): %v", table, pkgLicenseCols, len(pkgLicenseValues), err)
			}
		}

		if len(importValues) > 0 {
			importCols := []string{
				"from_path",
				"from_module_path",
				"from_version",
				"to_path",
				"to_name",
			}
			table := "imports"
			if err := bulkInsert(ctx, tx, table, importCols, importValues, onConflictDoNothing); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d importValues): %v", table, importCols, len(importValues), err)
			}
		}
		return nil
	})
}

// validateVersion checks that fields needed to insert a version into the
// database are present. Otherwise, it returns an error listing the reasons the
// version cannot be inserted.
func validateVersion(version *internal.Version) error {
	if version == nil {
		return fmt.Errorf("nil version")
	}

	var errReasons []string
	if version.SeriesPath == "" {
		errReasons = append(errReasons, "no series path")
	}
	if version.Version == "" {
		errReasons = append(errReasons, "no specified version")
	} else if version.ModulePath != "std" && !semver.IsValid(version.Version) {
		errReasons = append(errReasons, "invalid version")
	}
	if version.ModulePath == "" {
		errReasons = append(errReasons, "no module path")
	} else if err := module.CheckPath(version.ModulePath); err != nil && version.ModulePath != "std" {
		errReasons = append(errReasons, "invalid module path")
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
