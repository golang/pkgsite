// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fakedatasource

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetLatestInfo_MajorPath(t *testing.T) {
	type testModule struct {
		path    string
		version string
		suffix  string
	}
	testCases := []struct {
		modules             []testModule
		modulePath          string
		unitPath            string
		wantMajorModulePath string
		wantMajorUnitPath   string
	}{
		{
			modules: []testModule{
				{path: "example.com/mod", version: "v1.0.0", suffix: "a"},
				{path: "example.com/mod/v2", version: "v2.0.0", suffix: "a/b"},
			},
			modulePath:          "example.com/mod",
			unitPath:            "example.com/mod/a/b",
			wantMajorModulePath: "example.com/mod/v2",
			wantMajorUnitPath:   "example.com/mod/v2/a/b",
		},
		{
			modules: []testModule{
				{path: "example.com/mod", version: "v1.0.0", suffix: "a"},
				{path: "example.com/mod/v2", version: "v2.0.0", suffix: "a"},
			},
			modulePath:          "example.com/mod",
			unitPath:            "example.com/mod/a/b",
			wantMajorModulePath: "example.com/mod/v2",
			wantMajorUnitPath:   "example.com/mod/v2",
		},
		{
			modules: []testModule{
				{path: "example.com/mod", version: "v1.0.0", suffix: "a"},
				{path: "example.com/mod/v2", version: "v2.0.0", suffix: "a"},
				{path: "example.com/mod/v2", version: "v2.1.0", suffix: "a/b"},
			},
			modulePath:          "example.com/mod",
			unitPath:            "example.com/mod/a/b",
			wantMajorModulePath: "example.com/mod/v2",
			wantMajorUnitPath:   "example.com/mod/v2/a/b",
		},
		{
			modules: []testModule{
				{path: "example.com/mod", version: "v1.0.0", suffix: "a"},
				{path: "example.com/mod/v2", version: "v2.0.0", suffix: "a/b"},
				{path: "example.com/mod/v3", version: "v3.0.0", suffix: "a"},
			},
			modulePath:          "example.com/mod",
			unitPath:            "example.com/mod/a/b",
			wantMajorModulePath: "example.com/mod/v3",
			wantMajorUnitPath:   "example.com/mod/v3",
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		fds := New()
		for _, m := range tc.modules {
			fds.MustInsertModule(t, sample.Module(m.path, m.version, m.suffix))
		}
		latest, err := fds.GetLatestInfo(ctx, tc.unitPath, tc.modulePath, nil)
		if err != nil {
			t.Errorf("fds.GetLatestInfo(%q, %q): got error %v; expected none", tc.modulePath, tc.unitPath, err)
			continue
		}
		if latest.MajorModulePath != tc.wantMajorModulePath {
			t.Errorf("fds.GetLatestInfo(%q, %q).MajorModulePath: got %q, want %q", tc.modulePath, tc.unitPath, latest.MajorModulePath, tc.wantMajorModulePath)
		}
		if latest.MajorUnitPath != tc.wantMajorUnitPath {
			t.Errorf("fds.GetLatestInfo(%q, %q).MajorUnitPath: got %q, want %q", tc.modulePath, tc.unitPath, latest.MajorUnitPath, tc.wantMajorUnitPath)
		}
	}
}

func TestGetImportedByCounts(t *testing.T) {
	ctx := context.Background()
	fds := New()

	// m1.com has the target package and an internal importer.
	m1 := sample.Module("m1.com", "v1.0.0", "target", "internal")
	m1.Packages()[1].Imports = []string{"m1.com/target"}
	fds.MustInsertModule(t, m1)

	// m2.com has two packages that both import m1.com/target.
	m2 := sample.Module("m2.com", "v1.0.0", "p1", "p2")
	m2.Packages()[0].Imports = []string{"m1.com/target"}
	m2.Packages()[1].Imports = []string{"m1.com/target"}
	fds.MustInsertModule(t, m2)

	// m3.com imported m1.com/target at v1.0.0, but not at latest v1.1.0.
	m3Old := sample.Module("m3.com", "v1.0.0", "p1")
	m3Old.Packages()[0].Imports = []string{"m1.com/target"}
	fds.MustInsertModule(t, m3Old)
	m3New := sample.Module("m3.com", "v1.1.0", "p1")
	fds.MustInsertModule(t, m3New)

	got, err := fds.GetImportedByCounts(ctx, "m1.com/target", "m1.com")
	if err != nil {
		t.Fatal(err)
	}
	// Packages excludes internal imports (m1.com/internal) and non-latest versions (m3.com@v1.0.0),
	// so only m2.com/p1 and m2.com/p2 count.
	if got.Packages != 2 {
		t.Errorf("Packages = %d, want 2", got.Packages)
	}
	// Modules includes self-imports (m1.com) and deduplicates packages in m2.com.
	if got.Modules != 2 {
		t.Errorf("Modules = %d, want 2", got.Modules)
	}
}

func TestGetImporterRank(t *testing.T) {
	ctx := t.Context()
	fds := New()

	// Target packages A, B, C, D are in m1.com.
	m1 := sample.Module("m1.com", "v1.0.0", "pkgA", "pkgB", "pkgC", "pkgD")
	fds.MustInsertModule(t, m1)

	// m2 imports pkgA, pkgB, pkgC.
	m2 := sample.Module("m2.com", "v1.0.0", "p1")
	m2.Packages()[0].Imports = []string{"m1.com/pkgA", "m1.com/pkgB", "m1.com/pkgC"}
	fds.MustInsertModule(t, m2)

	// m3 imports pkgA, pkgB, pkgC.
	m3 := sample.Module("m3.com", "v1.0.0", "p1")
	m3.Packages()[0].Imports = []string{"m1.com/pkgA", "m1.com/pkgB", "m1.com/pkgC"}
	fds.MustInsertModule(t, m3)

	// m4 imports pkgA.
	m4 := sample.Module("m4.com", "v1.0.0", "p1")
	m4.Packages()[0].Imports = []string{"m1.com/pkgA"}
	fds.MustInsertModule(t, m4)

	// pkgA: imported by m2, m3, m4 -> 3 modules
	// pkgB: imported by m2, m3 -> 2 modules
	// pkgC: imported by m2, m3 -> 2 modules (tied with B)
	// pkgD: imported by none -> 0 modules
	// Ranks among packages:
	// pkgA: 1 (0 higher)
	// pkgB: 2 (1 higher: pkgA)
	// pkgC: 2 (1 higher: pkgA)
	// pkgD: 4 (3 higher: pkgA, pkgB, pkgC)

	tests := []struct {
		pkgPath  string
		wantRank int
	}{
		{"m1.com/pkgA", 1},
		{"m1.com/pkgB", 2},
		{"m1.com/pkgC", 2},
		{"m1.com/pkgD", 4},
		{"nonexistent", 0},
	}
	for _, tc := range tests {
		got, err := fds.GetImporterRank(ctx, tc.pkgPath)
		if err != nil {
			t.Errorf("GetImporterRank(ctx, %q): %v", tc.pkgPath, err)
			continue
		}
		if got != tc.wantRank {
			t.Errorf("GetImporterRank(ctx, %q) = %d, want %d", tc.pkgPath, got, tc.wantRank)
		}
	}

	// An uncomputed package (NumImportedByModules < 0) returns 0.
	mUncomputed := sample.Module("uncomputed.com", "v1.0.0", "pkg")
	mUncomputed.Packages()[0].NumImportedByModules = -1
	fds.MustInsertModule(t, mUncomputed)
	got, err := fds.GetImporterRank(ctx, "uncomputed.com/pkg")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("GetImporterRank(uncomputed) = %d, want 0", got)
	}
}
