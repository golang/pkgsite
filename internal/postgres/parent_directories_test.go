// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestToTsvectorParentDirectoriesStoredProcedure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, tc := range []struct {
		path, modulePath string
		want             []string
	}{
		{
			path:       "github.com/a/b",
			modulePath: "github.com/a/b",
			want:       []string{"github.com/a/b"},
		},
		{
			path:       "github.com/a/b/c",
			modulePath: "github.com/a/b",
			want: []string{
				"github.com/a/b",
				"github.com/a/b/c",
			},
		},
		{
			path:       "github.com/a/b/c/github.com/a/b/c",
			modulePath: "github.com/a/b",
			want: []string{
				"github.com/a/b",
				"github.com/a/b/c",
				"github.com/a/b/c/github.com",
				"github.com/a/b/c/github.com/a",
				"github.com/a/b/c/github.com/a/b",
				"github.com/a/b/c/github.com/a/b/c",
			},
		},
		{
			path:       "bufio",
			modulePath: "std",
			want: []string{
				"bufio",
			},
		},
		{
			path:       "archive/zip",
			modulePath: "std",
			want: []string{
				"archive",
				"archive/zip",
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			m := sample.Module()
			m.ModulePath = tc.modulePath
			pkg := sample.Package()
			pkg.Path = tc.path
			m.Packages = []*internal.Package{pkg}
			if err := testDB.InsertModule(ctx, m); err != nil {
				t.Fatal(err)
			}

			var got []string
			err := testDB.db.QueryRow(ctx,
				`SELECT tsvector_to_array(tsv_parent_directories) FROM packages WHERE path = $1;`,
				tc.path).Scan(pq.Array(&got))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("tsvector_to_array FROM packages for %q mismatch (-want +got):\n%s", tc.path, diff)
			}

			err = testDB.db.QueryRow(ctx,
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
