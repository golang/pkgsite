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

