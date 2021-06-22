// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestSymbolSearch(t *testing.T) {
	ctx := context.Background()
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbolSearchDocuments)
	testDB, release := acquire(t)
	defer release()

	m := sample.DefaultModule()
	m.Packages()[0].Documentation[0].API = sample.API
	MustInsertModule(ctx, t, testDB, m)
	for _, test := range []struct {
		name string
		q    string
	}{
		// "V" is the only symbol in sample.DocContents.
		{
			"test search by <package>.<identifier>",
			fmt.Sprintf("%s.%s", sample.PackageName, "V"),
		},
		{
			"test search by <identifier>",
			"V",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp, err := testDB.hedgedSearch(ctx, test.q, 2, 0, 100, symbolSearchers, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.results) == 0 {
				t.Fatalf("expected results")
			}
			for _, r := range resp.results {
				want := &internal.SearchResult{
					Name:           sample.PackageName,
					PackagePath:    sample.PackagePath,
					ModulePath:     sample.ModulePath,
					Version:        sample.VersionString,
					Synopsis:       m.Packages()[0].Documentation[0].Synopsis,
					Licenses:       []string{"MIT"},
					CommitTime:     sample.CommitTime,
					NumResults:     1,
					SymbolName:     "V",
					SymbolKind:     internal.SymbolKindVariable,
					SymbolSynopsis: "var V int",
					SymbolGOOS:     internal.All,
					SymbolGOARCH:   internal.All,
				}
				if diff := cmp.Diff(want, r); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
