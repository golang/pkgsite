// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
)

func TestInsertDocumentsAndSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now        = time.Now()
		versionFoo = &internal.Version{
			Version:     "v1.0.0",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			ModulePath:  "github.com/valid_module_name",
			SeriesPath:  "myseries",
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Path:     "/path/to/foo",
					Synopsis: "foo",
				},
			},
		}
		versionBar = &internal.Version{
			Version:     "v1.0.0",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			ModulePath:  "github.com/valid_module_name",
			SeriesPath:  "myseries",
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "bar",
					Path:     "/path/to/bar",
					Synopsis: "bar is bar", // add an extra 'bar' to make sorting deterministic
				},
			},
		}
		pkgFoo = &internal.Package{
			Name:     "foo",
			Path:     "/path/to/foo",
			Synopsis: "foo",
			Version: &internal.Version{
				ModulePath: "github.com/valid_module_name",
				Version:    "v1.0.0",
			},
		}
		pkgBar = &internal.Package{
			Name:     "bar",
			Path:     "/path/to/bar",
			Synopsis: "bar is bar",
			Version: &internal.Version{
				ModulePath: "github.com/valid_module_name",
				Version:    "v1.0.0",
			},
		}
	)

	for _, tc := range []struct {
		name                 string
		terms                []string
		versions             []*internal.Version
		want                 []*SearchResult
		insertErr, searchErr derrors.ErrorType
	}{
		{
			name:     "two_documents_different_packages_multiple_terms",
			terms:    []string{"foo", "bar"},
			versions: []*internal.Version{versionFoo, versionBar},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.15107775919689598,
					NumImportedBy: 0,
					Total:         2,
					Package:       pkgBar,
				},
				&SearchResult{
					Rank:          0.14521065270483344,
					NumImportedBy: 0,
					Total:         2,
					Package:       pkgFoo,
				},
			},
		},
		{
			name:     "two_documents_different_packages_one_result",
			terms:    []string{"foo"},
			versions: []*internal.Version{versionFoo, versionBar},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.2904213054096669,
					NumImportedBy: 0,
					Total:         1,
					Package:       pkgFoo,
				},
			},
		},
		{
			name:     "one_document",
			terms:    []string{"foo"},
			versions: []*internal.Version{versionFoo},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.2904213054096669,
					NumImportedBy: 0,
					Total:         1,
					Package:       pkgFoo,
				},
			},
		},
		{
			name:      "no_documents",
			terms:     []string{},
			versions:  nil,
			want:      nil,
			insertErr: derrors.InvalidArgumentType,
			searchErr: derrors.InvalidArgumentType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			if tc.versions != nil {
				for _, v := range tc.versions {
					if err := db.InsertVersion(ctx, v, nil); derrors.Type(err) != tc.insertErr {
						t.Fatalf("db.InsertVersion(%+v): %v", tc.versions, err)
					}
					if err := db.InsertDocuments(ctx, v); derrors.Type(err) != tc.insertErr {
						t.Fatalf("db.InsertDocuments(%+v): %v", tc.versions, err)
					}
				}
			}

			got, err := db.Search(ctx, tc.terms)
			if derrors.Type(err) != tc.searchErr {
				t.Fatalf("db.Search(ctx, %v): %v", tc.terms, err)
			}
			if len(got) != len(tc.want) {
				t.Errorf("db.Search(ctx, %v) mismatch: len(got) = %d, want = %d\n", tc.terms, len(got), len(tc.want))
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(internal.Version{}, "CommitTime")); diff != "" {
				t.Errorf("db.Search(ctx, %v) mismatch (-want +got):\n%s", tc.terms, diff)
			}
		})
	}
}
