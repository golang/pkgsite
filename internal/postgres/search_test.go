// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
)

func TestInsertDocumentsAndSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		versionFoo = sampleVersion(func(v *internal.Version) {
			v.Packages = []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Path:     "/path/to/foo",
					Synopsis: "foo",
					Licenses: sampleLicenseInfos,
				},
			}
		})
		versionBar = sampleVersion(func(v *internal.Version) {
			v.Packages = []*internal.Package{
				&internal.Package{
					Name:     "bar",
					Path:     "/path/to/bar",
					Synopsis: "bar is bar", // add an extra 'bar' to make sorting deterministic
					Licenses: sampleLicenseInfos,
				},
			}
		})

		pkgFoo = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "foo",
				Path:     "/path/to/foo",
				Synopsis: "foo",
				Licenses: sampleLicenseInfos,
			},
			VersionInfo: internal.VersionInfo{
				ModulePath: versionFoo.ModulePath,
				Version:    versionFoo.Version,
				CommitTime: versionFoo.CommitTime,
			},
		}
		pkgBar = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "bar",
				Path:     "/path/to/bar",
				Synopsis: "bar is bar",
				Licenses: sampleLicenseInfos,
			},
			VersionInfo: internal.VersionInfo{
				ModulePath: versionBar.ModulePath,
				Version:    versionBar.Version,
				CommitTime: versionBar.CommitTime,
			},
		}
	)

	for _, tc := range []struct {
		name                 string
		terms                []string
		versions             []*internal.Version
		want                 []*SearchResult
		insertErr, searchErr derrors.ErrorType
		limit, offset        int
	}{
		{
			name:     "two_documents_different_packages_multiple_terms",
			terms:    []string{"foo", "bar"},
			versions: []*internal.Version{versionFoo, versionBar},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.15107775919689598,
					NumImportedBy: 0,
					NumResults:    2,
					Package:       pkgBar,
				},
				&SearchResult{
					Rank:          0.14521065270483344,
					NumImportedBy: 0,
					NumResults:    2,
					Package:       pkgFoo,
				},
			},
		},
		{
			name:     "two_documents_different_packages_multiple_terms_limit_1_offset_0",
			terms:    []string{"foo", "bar"},
			limit:    1,
			offset:   0,
			versions: []*internal.Version{versionFoo, versionBar},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.15107775919689598,
					NumImportedBy: 0,
					Package:       pkgBar,
					NumResults:    2,
				},
			},
		},
		{
			name:     "two_documents_different_packages_multiple_terms_limit_1_offset_1",
			terms:    []string{"foo", "bar"},
			limit:    1,
			offset:   1,
			versions: []*internal.Version{versionFoo, versionBar},
			want: []*SearchResult{
				&SearchResult{
					Rank:          0.14521065270483344,
					NumImportedBy: 0,
					Package:       pkgFoo,
					NumResults:    2,
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
					NumResults:    1,
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
					NumResults:    1,
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
			defer ResetTestDB(testDB, t)

			if tc.versions != nil {
				for _, v := range tc.versions {
					if err := testDB.InsertVersion(ctx, v, sampleLicenses); derrors.Type(err) != tc.insertErr {
						t.Fatalf("testDB.InsertVersion(%+v): %v", tc.versions, err)
					}
					if err := testDB.InsertDocuments(ctx, v); derrors.Type(err) != tc.insertErr {
						t.Fatalf("testDB.InsertDocuments(%+v): %v", tc.versions, err)
					}
				}
			}

			if tc.limit < 1 {
				tc.limit = 10
			}
			got, err := testDB.Search(ctx, tc.terms, tc.limit, tc.offset)
			if derrors.Type(err) != tc.searchErr {
				t.Fatalf("testDB.Search(%v, %d, %d): %v", tc.terms, tc.limit, tc.offset, err)
			}

			if len(got) != len(tc.want) {
				t.Errorf("testDB.Search(%v, %d, %d) mismatch: len(got) = %d, want = %d\n", tc.terms, tc.limit, tc.offset, len(got), len(tc.want))
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("testDB.Search(%v, %d, %d) mismatch (-want +got):\n%s", tc.terms, tc.limit, tc.offset, diff)
			}
		})
	}
}
