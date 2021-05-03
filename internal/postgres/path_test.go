// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetLatestMajorPathForV1Path(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, test := range []struct {
		name           string
		v1ModulePath   string
		modvers        []string
		wantModulePath string
		wantVersion    int
	}{
		{
			"want highest major version",
			"m.com",
			[]string{"m.com@v1.0.0", "m.com/v2@v2.0.0", "m.com/v11@v11.0.0"},
			"m.com/v11", 11,
		},
		{
			"only v1 version",
			"m.com",
			[]string{"m.com@v1.0.0"},
			"m.com", 1,
		},
		{
			"no v1 version",
			"m.com",
			[]string{"m.com/v4@v4.0.0"},
			"m.com/v4", 4,
		},
		{
			"gopkg.in",
			"gopkg.in/yaml",
			[]string{"gopkg.in/yaml.v1@v1.0.0", "gopkg.in/yaml.v2@v2.0.0"},
			"gopkg.in/yaml.v2", 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			const suffix = "a/b/c"

			check := func(v1path, wantPath string) {
				t.Helper()
				gotPath, gotVer, err := testDB.GetLatestMajorPathForV1Path(ctx, v1path)
				if err != nil {
					t.Fatal(err)
				}
				if gotPath != wantPath || gotVer != test.wantVersion {
					t.Errorf("GetLatestMajorPathForV1Path(%q) = %q, %d, want %q, %d",
						v1path, gotPath, gotVer, wantPath, test.wantVersion)
				}
			}

			for _, mv := range test.modvers {
				mod, ver, _ := parseModuleVersionPackage(mv)
				m := sample.Module(mod, ver, suffix)
				MustInsertModule(ctx, t, testDB, m)
			}
			t.Run("module", func(t *testing.T) {
				check(test.v1ModulePath, test.wantModulePath)
			})
			t.Run("package", func(t *testing.T) {
				check(test.v1ModulePath+"/"+suffix, test.wantModulePath+"/"+suffix)
			})
		})
	}
}

func TestUpsertPathConcurrently(t *testing.T) {
	// Verify that we get no constraint violations or other errors when
	// the same path is upserted multiple times concurrently.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const n = 10
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			errc <- testDB.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
				id, err := upsertPath(ctx, tx, "a/path")
				if err != nil {
					return err
				}
				if id == 0 {
					return errors.New("zero id")
				}
				return nil
			})
		}()

	}
	for i := 0; i < n; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpsertPaths(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	check := func(paths []string) {
		got, err := upsertPathsInTx(ctx, testDB.db, paths)
		if err != nil {
			t.Fatal(err)
		}
		checkPathMap(t, got, paths)
	}

	check([]string{"a", "b", "c"})
	check([]string{"b", "c", "d", "e"})
}

func checkPathMap(t *testing.T, got map[string]int, paths []string) {
	t.Helper()
	if g, w := len(got), len(paths); g != w {
		t.Errorf("got %d paths, want %d", g, w)
		return
	}
	for _, p := range paths {
		g, ok := got[p]
		if !ok {
			t.Errorf("missing path %q", p)
		} else if g == 0 {
			t.Errorf("path %q has a 0 ID", p)
		}
	}
}

func TestUpsertPathsConcurrently(t *testing.T) {
	// Verify that we get no constraint violations or other errors when
	// the same set of paths is upserted multiple times concurrently.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const n = 10
	paths := make([]string, 100)
	for i := 0; i < len(paths); i++ {
		paths[i] = fmt.Sprintf("p%d", i)
	}
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			start := (10 * i) % len(paths)
			end := start + 50
			if end > len(paths) {
				end = len(paths)
			}
			sub := paths[start:end]
			got, err := upsertPathsInTx(ctx, testDB.db, sub)
			if err == nil {
				checkPathMap(t, got, sub)

			}
			errc <- err
		}()

	}
	for i := 0; i < n; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func upsertPathsInTx(ctx context.Context, db *database.DB, paths []string) (map[string]int, error) {
	var m map[string]int
	err := db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		var err error
		m, err = upsertPaths(ctx, tx, paths)
		return err
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}
