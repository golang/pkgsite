// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"context"
	"sort"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v7"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal/complete"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/testing/sample"
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
	v1 := sample.Version()
	v1.ModulePath = "github.com/something"
	v1.Packages[0].Path = v1.ModulePath + "/apples/bananas"
	v2 := sample.Version()
	v2.ModulePath = "github.com/something/else"
	v2.Packages[0].Path = v2.ModulePath + "/oranges/bananas"
	v2.Packages[0].Imports = []string{v1.Packages[0].Path}
	if err := testDB.InsertVersion(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if err := testDB.InsertVersion(ctx, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.UpdateSearchDocumentsImportedByCount(ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateRedisIndexes(ctx, testDB.GetSQLDB(), rc, 1); err != nil {
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

func TestPathCompletions(t *testing.T) {
	partial := complete.Completion{
		ModulePath:  "my.module/foo",
		PackagePath: "my.module/foo/bar",
		Version:     "v1.2.3",
		Importers:   123,
	}
	completions := pathCompletions(partial)
	sort.Slice(completions, func(i, j int) bool {
		return len(completions[i].Suffix) < len(completions[j].Suffix)
	})
	wantSuffixes := []string{"bar", "foo/bar", "my.module/foo/bar"}
	if got, want := len(completions), len(wantSuffixes); got != want {
		t.Fatalf("len(pathCompletions(%v)) = %d, want %d", partial, got, want)
	}
	for i, got := range completions {
		want := partial
		want.Suffix = wantSuffixes[i]
		if diff := cmp.Diff(want, *got); diff != "" {
			t.Errorf("completions[%d] mismatch (-want +got)\n%s", i, diff)
		}
	}
}

func TestPathSuffixes(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"foo/Bar/baz", []string{"foo/bar/baz", "bar/baz", "baz"}},
		{"foo", []string{"foo"}},
		{"BAR", []string{"bar"}},
	}
	for _, test := range tests {
		if got := pathSuffixes(test.path); !cmp.Equal(got, test.want) {
			t.Errorf("prefixes(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}
