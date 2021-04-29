// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Functions for cleaning the database of unwanted module versions.

package postgres

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetModuleVersionsToClean(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	want := []string{
		"a.c@v0.0.0-20190101000000-abcdef012345",
	}
	for _, mv := range append(want,
		// These should not be cleaned.
		"a.c@v1.0.0", // tagged
		"b.c@v0.0.0-20170101000000-abcdef012345/p", // p is in search_documents (must be inserted first)
		"b.c@v0.0.0-20190101000000-abcdef012345",   // latest version
		"b.c@v0.0.0-20180101000000-abcdef012345",   // 'main' in version_map (see UpsertVersionMap below)
	) {
		mod, ver, pkg := parseModuleVersionPackage(mv)
		m := sample.Module(mod, ver, pkg)
		MustInsertModule(ctx, t, testDB, m)
	}
	if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
		ModulePath:       "b.c",
		RequestedVersion: "main",
		ResolvedVersion:  "v0.0.0-20180101000000-abcdef012345",
		Status:           200,
	}); err != nil {
		t.Fatal(err)
	}

	mvs, err := testDB.GetModuleVersionsToClean(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, mv := range mvs {
		got = append(got, mv.String())
	}
	sort.Strings(got)
	if !cmp.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}
