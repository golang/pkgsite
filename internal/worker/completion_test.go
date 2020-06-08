// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v7"
	"golang.org/x/pkgsite/internal/complete"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestUpdateRedisIndexes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	// Set up a simple test case with two module versions and two packages. The
	// package in v2 imports the package in v1. By setting our 'popular cutoff'
	// to 1, we can force the package in v1 to be considered popular.
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	m1 := sample.Module("github.com/something", sample.VersionString, "apples/bananas")
	m2 := sample.Module("github.com/something/else", sample.VersionString, "oranges/bananas")
	m2.LegacyPackages[0].Imports = []string{m1.LegacyPackages[0].Path}
	if err := testDB.InsertModule(ctx, m1); err != nil {
		t.Fatal(err)
	}
	if err := testDB.InsertModule(ctx, m2); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateRedisIndexes(ctx, testDB.Underlying(), rc, 1); err != nil {
		t.Fatal(err)
	}
	popCount, err := rc.ZCount(complete.PopularKey, "0", "0").Result()
	if err != nil {
		t.Fatal(err)
	}
	// There are 4 path components in github.com/something/apples/bananas
	if popCount != 4 {
		t.Errorf("got %d popular autocompletions, want %d", popCount, 4)
	}
	remCount, err := rc.ZCount(complete.RemainingKey, "0", "0").Result()
	if err != nil {
		t.Fatal(err)
	}
	// There are 5 path components in github.com/something/else/oranges/bananas
	if remCount != 5 {
		t.Errorf("got %d remaining autocompletions, want %d", remCount, 5)
	}
}
