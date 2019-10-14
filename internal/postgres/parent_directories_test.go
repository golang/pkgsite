// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/sample"
)

func TestToTsvectorParentDirectoriesStoredProcedure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	v := sample.Version()
	v.ModulePath = "github.com/a/b"
	v.Version = "v1.0.0"
	v.Packages = []*internal.Package{}
	path1 := "github.com/a/b"
	path2 := "github.com/a/b/c"
	path3 := "github.com/a/b/c/github.com/a/b/c"
	for _, p := range []string{path1, path2, path3} {
		pkg := sample.Package()
		pkg.Path = p
		v.Packages = append(v.Packages, pkg)
	}
	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		want []string
	}{
		{
			path: path1,
			want: []string{path1},
		},
		{
			path: path2,
			want: []string{path1, path2},
		},
		{
			path: path3,
			want: []string{
				"github.com/a/b",
				"github.com/a/b/c",
				"github.com/a/b/c/github.com",
				"github.com/a/b/c/github.com/a",
				"github.com/a/b/c/github.com/a/b",
				"github.com/a/b/c/github.com/a/b/c",
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var got []string
			err := testDB.queryRow(ctx,
				`SELECT tsvector_to_array(tsv_parent_directories) FROM packages WHERE path = $1;`,
				tc.path).Scan(pq.Array(&got))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("tsvector_to_array FROM packages for %q mismatch (-want +got):\n%s", tc.path, diff)
			}

			err = testDB.queryRow(ctx,
				`SELECT tsvector_to_array(tsv_parent_directories) FROM search_documents WHERE package_path = $1;`,
				tc.path).Scan(pq.Array(&got))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("tsvector_to_array FROM search_documents for %q mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}
