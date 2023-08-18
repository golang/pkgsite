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
			fds.MustInsertModule(ctx, sample.Module(m.path, m.version, m.suffix))
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
