// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// versionsDiff takes in two versions, v1 and v2, and returns a string
// description of the difference between them. If they are the
// same the string will be empty. Fields "CreatedAt",
// "UpdatedAt", "Module.Series", "VersionType", "Packages",
// and "Module.Versions" are ignored during comparison.
func versionsDiff(v1, v2 *internal.Version) string {
	if v1 == nil && v2 == nil {
		return ""
	}

	if (v1 != nil && v2 == nil) || (v1 == nil && v2 != nil) {
		return "not equal"
	}

	return cmp.Diff(*v1, *v2, cmpopts.IgnoreFields(internal.Version{},
		"CreatedAt", "UpdatedAt", "Module.Series", "VersionType", "Packages", "Module.Versions"))
}

// packagesDiff takes two packages, p1 and p2, and returns a string description
// of the difference between them. If they are the same they will be
// empty.
func packagesDiff(p1, p2 *internal.Package) string {
	if p1 == nil && p2 == nil {
		return ""
	}

	if (p1 != nil && p2 == nil) || (p1 == nil && p2 != nil) {
		return "not equal"
	}

	return fmt.Sprintf("%v%v", cmp.Diff(*p1, *p2, cmpopts.IgnoreFields(internal.Package{}, "Version")),
		versionsDiff(p1.Version, p2.Version))
}

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	var (
		now    = time.Now()
		series = &internal.Series{
			Path:    "myseries",
			Modules: []*internal.Module{},
		}
		module = &internal.Module{
			Path:     "github.com/valid_module_name",
			Series:   series,
			Versions: []*internal.Version{},
		}
		testVersion = &internal.Version{
			Module:     module,
			Version:    "v1.0.0",
			License:    "licensename",
			ReadMe:     []byte("readme"),
			CommitTime: now,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "path.to/foo",
				},
			},
			VersionType: internal.VersionTypeRelease,
		}
	)

	testCases := []struct {
		name, module, version, pkgpath string
		versionData                    *internal.Version
		wantWriteErrCode               codes.Code
		wantReadErr                    bool
	}{
		{
			name:             "nil_version_write_error",
			module:           "github.com/valid_module_name",
			version:          "v1.0.0",
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name:        "valid_test",
			module:      "github.com/valid_module_name",
			version:     "v1.0.0",
			pkgpath:     "path.to/foo",
			versionData: testVersion,
		},
		{
			name:        "nonexistent_version_test",
			module:      "github.com/valid_module_name",
			version:     "v1.2.3",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name:        "nonexistent_module_test",
			module:      "nonexistent_module_name",
			version:     "v1.0.0",
			pkgpath:     "path.to/foo",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name: "missing_module",
			versionData: &internal.Version{
				Version:    "v1.0.0",
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "missing_module_name",
			versionData: &internal.Version{
				Module:     &internal.Module{},
				Version:    "v1.0.0",
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "missing_version",
			versionData: &internal.Version{
				Module:     module,
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "empty_commit_time",
			versionData: &internal.Version{
				Module:   module,
				Version:  "v1.0.0",
				Synopsis: "This is a synopsis",
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			if err := db.InsertVersion(tc.versionData); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(%+v) error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := db.InsertVersion(tc.versionData); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(%+v) second insert error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			got, err := db.GetVersion(tc.module, tc.version)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("db.GetVersion(%q, %q) error: %v, want read error: %t", tc.module, tc.version, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("db.GetVersion(%q, %q) = %v, want %v",
					tc.module, tc.version, got, tc.versionData)
			}

			if diff := versionsDiff(got, tc.versionData); !tc.wantReadErr && diff != "" {
				t.Errorf("db.GetVersion(%q, %q) = %v, want %v | diff is %v",
					tc.module, tc.version, got, tc.versionData, diff)
			}

			gotPkg, err := db.GetPackage(tc.pkgpath, tc.version)
			if tc.versionData == nil || tc.versionData.Packages == nil || tc.pkgpath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("db.GetPackage(%q, %q) = %v, want %v", tc.pkgpath, tc.version, err, sql.ErrNoRows)
				}
				return
			}

			wantPkg := tc.versionData.Packages[0]
			if err != nil {
				t.Fatalf("db.GetPackage(%q, %q) = %v, want %v", tc.pkgpath, tc.version, gotPkg, wantPkg)
			}

			if gotPkg.Version.Version != tc.versionData.Version {
				t.Errorf("db.GetPackage(%q, %q) version.version = %v, want %v", tc.pkgpath, tc.version, gotPkg.Version.Version, tc.versionData.Version)
			}
			if gotPkg.Version.License != tc.versionData.License {
				t.Errorf("db.GetPackage(%q, %q) version.license = %v, want %v", tc.pkgpath, tc.version, gotPkg.Version.License, tc.versionData.License)

			}

			gotPkg.Version = nil
			if diff := cmp.Diff(*gotPkg, *wantPkg); diff != "" {
				t.Errorf("db.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.pkgpath, tc.version, diff)
			}
		})
	}
}

func TestPostgres_GetLatestPackage(t *testing.T) {
	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)
	var (
		now = time.Now()
		pkg = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
		}
		series = &internal.Series{
			Path: "myseries",
		}
		module = &internal.Module{
			Path:   "path.to/foo",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0-alpha.1",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0-20190311183353-d8887717615a",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePseudo,
			},
		}
	)

	testCases := []struct {
		name, path  string
		versions    []*internal.Version
		wantPkg     *internal.Package
		wantReadErr bool
	}{
		{
			name:     "want_second_package",
			path:     pkg.Path,
			versions: testVersions,
			wantPkg: &internal.Package{
				Name:     pkg.Name,
				Path:     pkg.Path,
				Synopsis: pkg.Synopsis,
				Version: &internal.Version{
					CreatedAt: testVersions[1].CreatedAt,
					UpdatedAt: testVersions[1].UpdatedAt,
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    testVersions[1].Version,
					Synopsis:   testVersions[1].Synopsis,
					CommitTime: testVersions[1].CommitTime,
					License:    testVersions[1].License,
				},
			},
		},
		{
			name:        "empty_path",
			path:        "",
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.versions {
				if err := db.InsertVersion(v); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			gotPkg, err := db.GetLatestPackage(tc.path)
			if (err != nil) != tc.wantReadErr {
				t.Errorf("db.GetLatestPackage(%q): %v", tc.path, err)
			}

			if diff := packagesDiff(gotPkg, tc.wantPkg); diff != "" {
				t.Errorf("db.GetLatestPackage(%q) = %v, want %v, diff is %v",
					tc.path, gotPkg, tc.wantPkg, diff)
			}
		})
	}
}

func TestPostgres_GetTaggedAndPseudoVersions(t *testing.T) {
	var (
		now = time.Now()
		pkg = &internal.Package{
			Path: "path.to/foo/bar",
			Name: "bar",
		}
		series = &internal.Series{
			Path: "myseries",
		}
		module = &internal.Module{
			Path:   "path.to/foo",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module,
				Version:     "v1.0.1-alpha.1",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.1",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0-20190311183353-d8887717615a",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePseudo,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.1-beta",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePrerelease,
			},
		}
	)

	testCases := []struct {
		name, path   string
		versions     []*internal.Version
		wantVersions []*internal.Version
		pseudo       bool
	}{
		{
			name:     "want_release_and_prerelease",
			path:     pkg.Path,
			versions: testVersions,
			wantVersions: []*internal.Version{
				&internal.Version{
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    "v1.0.1",
					CommitTime: now,
				},
				&internal.Version{
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    "v1.0.1-beta",
					CommitTime: now,
				},
				&internal.Version{
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    "v1.0.1-alpha.1",
					CommitTime: now,
				},
			},
		},
		{
			name:     "want_pseudo",
			path:     pkg.Path,
			versions: testVersions,
			wantVersions: []*internal.Version{
				&internal.Version{
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    "v1.0.0-20190311183353-d8887717615a",
					CommitTime: now,
				},
			},
			pseudo: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			for _, v := range tc.versions {
				if err := db.InsertVersion(v); err != nil {
					t.Errorf("db.InsertVersion(%+v) error: %v", v, err)
				}
			}

			var (
				got             []*internal.Version
				diffErr, lenErr string
				err             error
			)

			if tc.pseudo {
				got, err = db.GetPseudoVersions(tc.path)
				if err != nil {
					t.Fatalf("db.GetPseudoVersions(%v) error: %v", tc.path, err)
				}
				diffErr = "db.GetPseudoVersions(%q, %q) mismatch (-want +got):\n%s"
				lenErr = "db.GetPseudoVersions(%q, %q) returned list of length %v, wanted %v"
			} else {
				got, err = db.GetTaggedVersions(tc.path)
				if err != nil {
					t.Fatalf("db.GetTaggedVersions(%v) error: %v", tc.path, err)
				}
				diffErr = "db.GetTaggedVersions(%q, %q) mismatch (-want +got):\n%s"
				lenErr = "db.GetTaggedVersions(%q, %q) returned list of length %v, wanted %v"
			}

			if len(got) != len(tc.wantVersions) {
				t.Fatalf(lenErr, tc.path, err, len(got), len(tc.wantVersions))
			}

			for i, v := range got {
				if diff := versionsDiff(v, tc.wantVersions[i]); diff != "" {
					t.Errorf(diffErr, v, tc.wantVersions[i], diff)
				}
			}
		})
	}
}

func TestPostgres_GetLatestPackageForPaths(t *testing.T) {
	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)
	var (
		now  = time.Now()
		pkg1 = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
		}
		pkg2 = &internal.Package{
			Path:     "path2.to/foo/bar2",
			Name:     "bar2",
			Synopsis: "This is another package synopsis",
		}
		series = &internal.Series{
			Path: "myseries",
		}
		module1 = &internal.Module{
			Path:   "path2.to/foo",
			Series: series,
		}
		module2 = &internal.Module{
			Path:   "path2.to/foo",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0-alpha.1",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v1.0.0-20190311183353-d8887717615a",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypePseudo,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v1.0.1-beta",
				License:     "licensename",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypePrerelease,
			},
		}
	)

	tc := struct {
		paths       []string
		versions    []*internal.Version
		wantPkgs    []*internal.Package
		wantReadErr bool
	}{
		paths:    []string{pkg1.Path, pkg2.Path},
		versions: testVersions,
		wantPkgs: []*internal.Package{
			&internal.Package{
				Name:     pkg1.Name,
				Path:     pkg1.Path,
				Synopsis: pkg1.Synopsis,
				Version: &internal.Version{
					CreatedAt: testVersions[1].CreatedAt,
					UpdatedAt: testVersions[1].UpdatedAt,
					Module: &internal.Module{
						Path: module1.Path,
					},
					Version:    testVersions[1].Version,
					Synopsis:   testVersions[1].Synopsis,
					CommitTime: testVersions[1].CommitTime,
					License:    testVersions[1].License,
				},
			},
			&internal.Package{
				Name:     pkg2.Name,
				Path:     pkg2.Path,
				Synopsis: pkg2.Synopsis,
				Version: &internal.Version{
					CreatedAt: testVersions[3].CreatedAt,
					UpdatedAt: testVersions[3].UpdatedAt,
					Module: &internal.Module{
						Path: module1.Path,
					},
					Version:    testVersions[3].Version,
					Synopsis:   testVersions[3].Synopsis,
					CommitTime: testVersions[3].CommitTime,
					License:    testVersions[3].License,
				},
			},
		},
	}

	for _, v := range tc.versions {
		if err := db.InsertVersion(v); err != nil {
			t.Errorf("db.InsertVersion(%v): %v", v, err)
		}
	}

	gotPkgs, err := db.GetLatestPackageForPaths(tc.paths)
	if (err != nil) != tc.wantReadErr {
		t.Errorf("db.GetLatestPackageForPaths(%q): %v", tc.paths, err)
	}

	for i, gotPkg := range gotPkgs {
		if diff := packagesDiff(gotPkg, tc.wantPkgs[i]); diff != "" {
			t.Errorf("got %v at index %v, want %v, diff is %v",
				gotPkg, i, tc.wantPkgs[i], diff)
		}
	}
}

func TestPostgress_InsertVersionLogs(t *testing.T) {
	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)

	now := time.Now().UTC()
	newVersions := []*internal.VersionLog{
		&internal.VersionLog{
			ModulePath: "testModule",
			Version:    "v.1.0.0",
			CreatedAt:  now.Add(-10 * time.Minute),
			Source:     internal.VersionSourceProxyIndex,
		},
		&internal.VersionLog{
			ModulePath: "testModule",
			Version:    "v.1.1.0",
			CreatedAt:  now,
			Source:     internal.VersionSourceProxyIndex,
		},
		&internal.VersionLog{
			ModulePath: "testModule/v2",
			Version:    "v.2.0.0",
			CreatedAt:  now,
			Source:     internal.VersionSourceProxyIndex,
		},
	}

	if err := db.InsertVersionLogs(newVersions); err != nil {
		t.Errorf("db.InsertVersionLogs(newVersions) error: %v", err)
	}

	dbTime, err := db.LatestProxyIndexUpdate()
	if err != nil {
		t.Errorf("db.LatestProxyIndexUpdate error: %v", err)
	}

	// Postgres has less precision than a time.Time value. Truncate to account for it.
	if !dbTime.Truncate(time.Millisecond).Equal(now.Truncate(time.Millisecond)) {
		t.Errorf("db.LatestProxyIndexUpdate() = %v, want %v", dbTime, now)
	}
}

func TestPostgres_prefixZeroes(t *testing.T) {
	testCases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{
			name:  "add_16_zeroes",
			input: "1111",
			want:  "00000000000000001111",
		},
		{
			name:  "add_nothing_exactly_20",
			input: "11111111111111111111",
			want:  "11111111111111111111",
		},
		{
			name:  "add_20_zeroes_empty_string",
			input: "",
			want:  "00000000000000000000",
		},
		{
			name:    "input_longer_than_20_char",
			input:   "123456789123456789123456789",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := prefixZeroes(tc.input); got != tc.want {
				t.Errorf("prefixZeroes(%v) = %v, want %v, err = %v, wantErr = %v", tc.input, got, tc.want, err, tc.wantErr)
			}
		})
	}
}

func TestPostgres_isNum(t *testing.T) {
	testCases := []struct {
		name, input string
		want        bool
	}{
		{
			name:  "all_numbers",
			input: "1111",
			want:  true,
		},
		{
			name:  "number_letter_mix",
			input: "111111asdf1a1111111asd",
			want:  false,
		},
		{
			name:  "empty_string",
			input: "",
			want:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNum(tc.input); got != tc.want {
				t.Errorf("isNum(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPostgres_padPrerelease(t *testing.T) {
	testCases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{
			name:  "pad_one_field",
			input: "-alpha.1",
			want:  "alpha.00000000000000000001",
		},
		{
			name:  "no_padding",
			input: "beta",
			want:  "beta",
		},
		{
			name:  "pad_two_fields",
			input: "-gamma.11.theta.2",
			want:  "gamma.00000000000000000011.theta.00000000000000000002",
		},
		{
			name:  "empty_string",
			input: "",
			want:  "~",
		},
		{
			name:    "num_field_longer_than_20_char",
			input:   "-alpha.123456789123456789123456789",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := padPrerelease(tc.input); (err != nil) == tc.wantErr && got != tc.want {
				t.Errorf("padPrerelease(%v) = %v, want %v, err = %v, wantErr = %v", tc.input, got, tc.want, err, tc.wantErr)
			}
		})
	}
}
