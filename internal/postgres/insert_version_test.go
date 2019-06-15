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
)

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testCases := []struct {
		name string

		version *internal.Version

		// identifiers to use for fetch
		getVersion, getModule, getPkg string

		// error conditions
		wantWriteErrType derrors.ErrorType
		wantReadErr      bool
	}{
		{
			name:       "valid test",
			version:    sampleVersion(),
			getModule:  sampleModulePath,
			getVersion: sampleVersionString,
			getPkg:     "path.to/foo",
		},
		{
			name:             "nil version write error",
			getModule:        sampleModulePath,
			getVersion:       sampleVersionString,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name:        "nonexistent version",
			version:     sampleVersion(),
			getModule:   sampleModulePath,
			getVersion:  "v1.2.3",
			wantReadErr: true,
		},
		{
			name:        "nonexistent module",
			version:     sampleVersion(),
			getModule:   "nonexistent_module_name",
			getVersion:  "v1.0.0",
			getPkg:      "path.to/foo",
			wantReadErr: true,
		},
		{
			name: "missing module path",
			version: sampleVersion(func(v *internal.Version) {
				v.ModulePath = ""
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name: "missing version",
			version: sampleVersion(func(v *internal.Version) {
				v.Version = ""
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name: "empty commit time",
			version: sampleVersion(func(v *internal.Version) {
				v.CommitTime = time.Time{}
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name: "stdlib",
			version: sampleVersion(func(v *internal.Version) {
				v.ModulePath = "std"
				v.SeriesPath = "std"
				v.Version = "v1.12.5"
				v.Packages = []*internal.Package{{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sampleLicenseInfos,
					DocumentationHTML: []byte("This is the documentation HTML"),
				}}
			}),
			getModule:  "std",
			getVersion: "v1.12.5",
			getPkg:     "context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			if err := testDB.InsertVersion(ctx, tc.version, sampleLicenses); derrors.Type(err) != tc.wantWriteErrType {
				t.Errorf("testDB.InsertVersion(ctx, %+v) error: %v, want write error: %v", tc.version, err, tc.wantWriteErrType)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := testDB.InsertVersion(ctx, tc.version, sampleLicenses); derrors.Type(err) != tc.wantWriteErrType {
				t.Errorf("testDB.InsertVersion(ctx, %+v) second insert error: %v, want write error: %v", tc.version, err, tc.wantWriteErrType)
			}

			got, err := testDB.GetVersion(ctx, tc.getModule, tc.getVersion)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("testDB.GetVersion(ctx, %q, %q) error: %v, want read error: %t", tc.getModule, tc.getVersion, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("testDB.GetVersion(ctx, %q, %q) = %v, want %v", tc.getModule, tc.getVersion, got, tc.version)
			}

			if tc.version != nil {
				if diff := cmp.Diff(&tc.version.VersionInfo, got); !tc.wantReadErr && diff != "" {
					t.Errorf("testDB.GetVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.getModule, tc.getVersion, diff)
				}
			}

			gotPkg, err := testDB.GetPackage(ctx, tc.getPkg, tc.getVersion)
			if tc.version == nil || tc.version.Packages == nil || tc.getPkg == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("testDB.GetPackage(ctx, %q, %q) = %v, want %v", tc.getPkg, tc.getVersion, err, sql.ErrNoRows)
				}
				return
			}
			if err != nil {
				t.Errorf("testDB.GetPackage(ctx, %q, %q): %v", tc.getPkg, tc.getVersion, err)
			}

			wantPkg := tc.version.Packages[0]
			if err != nil {
				t.Fatalf("testDB.GetPackage(ctx, %q, %q) = %v, want %v", tc.getPkg, tc.getVersion, gotPkg, wantPkg)
			}

			if gotPkg.VersionInfo.Version != tc.version.Version {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.getPkg, tc.getVersion, gotPkg.VersionInfo.Version, tc.version.Version)
			}

			if diff := cmp.Diff(wantPkg, &gotPkg.Package, cmpopts.IgnoreFields(internal.Package{}, "Imports")); diff != "" {
				t.Errorf("testDB.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.getPkg, tc.getVersion, diff)
			}
		})
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
