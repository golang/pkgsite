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

	checkResult := func(metas ...internal.SymbolMeta) []*internal.SearchResult {
		var results []*internal.SearchResult
		for _, sm := range metas {
			results = append(results,
				&internal.SearchResult{
					Name:           sample.PackageName,
					PackagePath:    sample.PackagePath,
					ModulePath:     sample.ModulePath,
					Version:        sample.VersionString,
					Synopsis:       m.Packages()[0].Documentation[0].Synopsis,
					Licenses:       []string{"MIT"},
					CommitTime:     sample.CommitTime,
					NumResults:     uint64(len(metas)),
					SymbolName:     sm.Name,
					SymbolKind:     sm.Kind,
					SymbolSynopsis: sm.Synopsis,
					SymbolGOOS:     internal.All,
					SymbolGOARCH:   internal.All,
				})
		}
		return results
	}
	for _, test := range []struct {
		name string
		q    string
		want []*internal.SearchResult
	}{
		{
			name: "test search by <package>.<identifier>",
			q:    fmt.Sprintf("%s.%s", sample.PackageName, sample.Variable.Name),
			want: checkResult(sample.Variable.SymbolMeta),
		},
		{
			name: "test search by <identifier>",
			q:    sample.Variable.Name,
			want: checkResult(sample.Variable.SymbolMeta),
		},
		{
			name: "test search by <recv>",
			q:    "Method",
			want: checkResult(sample.Method),
		},
		{
			name: "test search by <package> <identifier>",
			q:    sample.PackageName + " function",
			want: checkResult(sample.Function.SymbolMeta),
		},
		{
			name: "test search by <package-subpath> <identifier>",
			q:    "module_name/foo function",
			want: checkResult(sample.Function.SymbolMeta),
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
			if diff := cmp.Diff(test.want, resp.results); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
