// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/xerrors"
)

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testCases := []struct {
		name string

		version *internal.Version

		// identifiers to use for fetch
		wantModulePath, wantVersion, wantPkgPath string

		// error conditions
		wantWriteErr error
		wantReadErr  bool
	}{
		{
			name:           "valid test",
			version:        sample.Version(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.PackagePath,
		},
		{
			name: "valid test with internal package",
			version: func() *internal.Version {
				v := sample.Version()
				p := sample.Package()
				p.Path = sample.ModulePath + "/internal/foo"
				v.Packages = []*internal.Package{p}
				return v
			}(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.ModulePath + "/internal/foo",
		},
		{
			name:           "nil version write error",
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name:           "nonexistent version",
			version:        sample.Version(),
			wantModulePath: sample.ModulePath,
			wantVersion:    "v1.2.3",
			wantReadErr:    true,
		},
		{
			name:           "nonexistent module",
			version:        sample.Version(),
			wantModulePath: "nonexistent_module_path",
			wantVersion:    "v1.0.0",
			wantPkgPath:    sample.PackagePath,
			wantReadErr:    true,
		},
		{
			name: "missing module path",
			version: func() *internal.Version {
				v := sample.Version()
				v.ModulePath = ""
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "missing version",
			version: func() *internal.Version {
				v := sample.Version()
				v.Version = ""
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "empty commit time",
			version: func() *internal.Version {
				v := sample.Version()
				v.CommitTime = time.Time{}
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "stdlib",
			version: func() *internal.Version {
				v := sample.Version()
				v.ModulePath = "std"
				v.Version = "v1.12.5"
				v.Packages = []*internal.Package{{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sample.LicenseMetadata,
					DocumentationHTML: []byte("This is the documentation HTML"),
				}}
				return v
			}(),
			wantModulePath: "std",
			wantVersion:    "v1.12.5",
			wantPkgPath:    "context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			if err := testDB.InsertVersion(ctx, tc.version); !xerrors.Is(err, tc.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := testDB.InsertVersion(ctx, tc.version); !xerrors.Is(err, tc.wantWriteErr) {
				t.Errorf("second insert error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			got, err := testDB.GetVersionInfo(ctx, tc.wantModulePath, tc.wantVersion)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("error: got %v, want read error: %t", err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("testDB.GetVersionInfo(ctx, %q, %q) = %v, want %v", tc.wantModulePath, tc.wantVersion, got, tc.version)
			}

			if tc.version != nil {
				if diff := cmp.Diff(&tc.version.VersionInfo, got, cmp.AllowUnexported(source.Info{})); !tc.wantReadErr && diff != "" {
					t.Errorf("testDB.GetVersionInfo(ctx, %q, %q) mismatch (-want +got):\n%s", tc.wantModulePath, tc.wantVersion, diff)
				}
			}

			gotPkg, err := testDB.GetPackage(ctx, tc.wantPkgPath, internal.UnknownModulePath, tc.wantVersion)
			if tc.version == nil || tc.version.Packages == nil || tc.wantPkgPath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("got %v, want %v", err, sql.ErrNoRows)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			wantPkg := tc.version.Packages[0]
			if gotPkg.VersionInfo.Version != tc.version.Version {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.wantPkgPath, tc.wantVersion, gotPkg.VersionInfo.Version, tc.version.Version)
			}

			if diff := cmp.Diff(wantPkg, &gotPkg.Package, cmpopts.IgnoreFields(internal.Package{}, "Imports")); diff != "" {
				t.Errorf("testDB.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.wantPkgPath, tc.wantVersion, diff)
			}
		})
	}
}

func TestPostgres_ReadAndWriteVersionOtherColumns(t *testing.T) {
	// Verify that InsertVersion correctly populates the columns in the versions
	// table that are not in the VersionInfo struct.
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	type other struct {
		major, minor, patch    int
		prerelease, seriesPath string
	}

	v := sample.Version()
	v.ModulePath = "github.com/user/repo/path/v2"
	v.Version = "v1.2.3-beta.4.a"

	want := other{
		major:      1,
		minor:      2,
		patch:      3,
		prerelease: "beta.00000000000000000004.a",
		seriesPath: "github.com/user/repo/path",
	}

	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	query := `
	SELECT
		major, minor, patch, prerelease, series_path
	FROM
		versions
	WHERE
		module_path = $1 AND version = $2`
	row := testDB.queryRow(ctx, query, v.ModulePath, v.Version)
	var got other
	if err := row.Scan(&got.major, &got.minor, &got.patch, &got.prerelease, &got.seriesPath); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
	}
}

func TestPostgres_DeleteVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	v := sample.Version()
	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetVersionInfo(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if err := testDB.DeleteVersion(ctx, nil, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetVersionInfo(ctx, v.ModulePath, v.Version); !xerrors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
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
			input: "v1.0.0-alpha.1",
			want:  "alpha.00000000000000000001",
		},
		{
			name:  "no_padding",
			input: "v1.0.0-beta",
			want:  "beta",
		},
		{
			name:  "pad_two_fields",
			input: "v1.0.0-gamma.11.theta.2",
			want:  "gamma.00000000000000000011.theta.00000000000000000002",
		},
		{
			name:  "empty_string",
			input: "v1.0.0",
			want:  "~",
		},
		{
			name:    "num_field_longer_than_20_char",
			input:   "v1.0.0-alpha.123456789123456789123456789",
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

func TestExtractSemverParts(t *testing.T) {
	for _, tc := range []struct {
		version, wantPrerelease         string
		wantMajor, wantMinor, wantPatch int
	}{
		{
			version:        "v1.5.2",
			wantMajor:      1,
			wantMinor:      5,
			wantPatch:      2,
			wantPrerelease: "~",
		},
		{
			version:        "v1.5.2",
			wantMajor:      1,
			wantMinor:      5,
			wantPatch:      2,
			wantPrerelease: "~",
		},
		{
			version:        "v1.5.2+incompatible",
			wantMajor:      1,
			wantMinor:      5,
			wantPatch:      2,
			wantPrerelease: "~",
		},
		{
			version:        "v1.5.2-alpha+buildtag",
			wantMajor:      1,
			wantMinor:      5,
			wantPatch:      2,
			wantPrerelease: "alpha",
		},
		{
			version:        "v1.5.2-alpha.1+buildtag",
			wantMajor:      1,
			wantMinor:      5,
			wantPatch:      2,
			wantPrerelease: "alpha.00000000000000000001",
		},
	} {
		t.Run(tc.version, func(t *testing.T) {
			gotMajor, gotMinor, gotPatch, gotPrerelease, err := extractSemverParts(tc.version)

			if err != nil {
				t.Errorf("extractSemverParts(%q): %v", tc.version, err)
			}
			if gotMajor != tc.wantMajor ||
				gotMinor != tc.wantMinor ||
				gotPatch != tc.wantPatch ||
				gotPrerelease != tc.wantPrerelease {
				t.Errorf("extractSemverParts(%q) = %d, %d, %d, %q; want = %d, %d, %d, %q", tc.version, gotMajor, gotMinor, gotPatch, gotPrerelease, tc.wantMajor, tc.wantMinor, tc.wantPatch, tc.wantPrerelease)
			}
		})
	}
}
