// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInsertDocumentsAndSearch(t *testing.T) {
	var (
		now     = time.Now()
		series1 = &internal.Series{
			Path: "myseries",
		}
		module1 = &internal.Module{
			Path:   "github.com/valid_module_name",
			Series: series1,
		}
	)

	for _, tc := range []struct {
		name                 string
		terms                []string
		versions             []*internal.Version
		want                 []*SearchResult
		insertErr, searchErr codes.Code
	}{
		{
			name:  "two_documents_different_packages_multiple_terms",
			terms: []string{"foo", "bar"},
			versions: []*internal.Version{
				&internal.Version{
					Version:     "v1.0.0",
					License:     "licensename",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
					Module:      module1,
					Packages: []*internal.Package{
						&internal.Package{
							Name:     "foo",
							Path:     "/path/to/foo",
							Synopsis: "foo",
						},
					},
				},
				&internal.Version{
					Version:     "v1.0.0",
					License:     "licensename",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
					Module:      module1,
					Packages: []*internal.Package{
						&internal.Package{
							Name:     "bar",
							Path:     "/path/to/bar",
							Synopsis: "bar is bar", // add an extra 'bar' to make sorting deterministic
						},
					},
				},
			},
			want: []*SearchResult{
				&SearchResult{
					Relevance:    0.3478693962097168,
					NumImporters: 0,
					Package: &internal.Package{
						Name:     "bar",
						Path:     "/path/to/bar",
						Synopsis: "bar is bar",
						Version: &internal.Version{
							Module: &internal.Module{
								Path: "github.com/valid_module_name",
							},
							Version: "v1.0.0",
							License: "licensename",
						},
					},
				},
				&SearchResult{
					Relevance:    0.33435988426208496,
					NumImporters: 0,
					Package: &internal.Package{
						Name:     "foo",
						Path:     "/path/to/foo",
						Synopsis: "foo",
						Version: &internal.Version{
							Module: &internal.Module{
								Path: "github.com/valid_module_name",
							},
							Version: "v1.0.0",
							License: "licensename",
						},
					},
				},
			},
		},

		{
			name:  "two_documents_different_packages_one_result",
			terms: []string{"foo"},
			versions: []*internal.Version{
				&internal.Version{
					Version:     "v1.0.0",
					License:     "licensename",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
					Module:      module1,
					Packages: []*internal.Package{
						&internal.Package{
							Name:     "foo",
							Path:     "/path/to/foo",
							Synopsis: "foo",
						},
					},
				},
				&internal.Version{
					Version:     "v1.0.0",
					License:     "licensename",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
					Module:      module1,
					Packages: []*internal.Package{
						&internal.Package{
							Name:     "bar",
							Path:     "/path/to/bar",
							Synopsis: "bar",
						},
					},
				},
			},
			want: []*SearchResult{
				&SearchResult{
					Relevance:    0.6687197685241699,
					NumImporters: 0,
					Package: &internal.Package{
						Name:     "foo",
						Path:     "/path/to/foo",
						Synopsis: "foo",
						Version: &internal.Version{
							Module: &internal.Module{
								Path: "github.com/valid_module_name",
							},
							Version: "v1.0.0",
							License: "licensename",
						},
					},
				},
			},
		},
		{
			name:  "one_document",
			terms: []string{"foo"},
			versions: []*internal.Version{
				&internal.Version{
					Version:     "v1.0.0",
					License:     "licensename",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
					Module:      module1,
					Packages: []*internal.Package{
						&internal.Package{
							Name:     "foo",
							Path:     "/path/to/foo",
							Synopsis: "foo",
						},
					},
				},
			},
			want: []*SearchResult{
				&SearchResult{
					Relevance:    0.6687197685241699,
					NumImporters: 0,
					Package: &internal.Package{
						Name:     "foo",
						Path:     "/path/to/foo",
						Synopsis: "foo",
						Version: &internal.Version{
							Module: &internal.Module{
								Path: "github.com/valid_module_name",
							},
							Version: "v1.0.0",
							License: "licensename",
						},
					},
				},
			},
		},
		{
			name:      "no_documents",
			terms:     []string{},
			versions:  nil,
			want:      nil,
			insertErr: codes.InvalidArgument,
			searchErr: codes.InvalidArgument,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			if tc.versions != nil {
				for _, v := range tc.versions {
					if err := db.InsertVersion(v); status.Code(err) != tc.insertErr {
						t.Fatalf("db.InsertVersion(%+v): %v", tc.versions, err)
					}
					if err := db.InsertDocuments(v); status.Code(err) != tc.insertErr {
						t.Fatalf("db.InsertDocuments(%+v): %v", tc.versions, err)
					}
				}
			}

			got, err := db.Search(tc.terms)
			if status.Code(err) != tc.searchErr {
				t.Fatalf("db.Search(%v): %v", tc.terms, err)
			}

			if len(got) != len(tc.want) {
				t.Errorf("db.Search(%v) mismatch: len(got) = %d, want = %d\n", tc.terms, len(got), len(tc.want))
			}

			for _, s := range got {
				if s.Package != nil && s.Package.Version != nil {
					s.Package.Version.CreatedAt = time.Time{}
					s.Package.Version.UpdatedAt = time.Time{}
					s.Package.Version.CommitTime = time.Time{}
				}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("db.Search(%v) mismatch (-want +got):\n%s", tc.terms, diff)
			}
		})
	}
}
