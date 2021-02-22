// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

func TestDeletePseudoversionsExcept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	pseudo1 := "v0.0.0-20190904010203-89fb59e2e920"
	versions := []string{
		sample.VersionString,
		pseudo1,
		"v0.0.0-20190904010203-89fb59e2e920",
		"v0.0.0-20190904010203-89fb59e2e920",
	}
	for _, v := range versions {
		MustInsertModule(ctx, t, testDB, sample.Module(sample.ModulePath, v, ""))
	}
	if err := testDB.DeletePseudoversionsExcept(ctx, sample.ModulePath, pseudo1); err != nil {
		t.Fatal(err)
	}
	mods, err := getPathVersions(ctx, testDB, sample.ModulePath, version.TypeRelease)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 && mods[0].Version != sample.VersionString {
		t.Errorf("module version %q was not found", sample.VersionString)
	}
	mods, err = getPathVersions(ctx, testDB, sample.ModulePath, version.TypePseudo)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("pseudoversions expected to be deleted were not")
	}
	if mods[0].Version != pseudo1 {
		t.Errorf("got %q; want %q", mods[0].Version, pseudo1)
	}
}
