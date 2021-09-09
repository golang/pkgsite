// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestSymbolSearch(t *testing.T) {
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	m := sample.DefaultModule()
	m.Packages()[0].Documentation[0].API = sample.API
	MustInsertModule(ctx, t, testDB, m)

	checkResult := func(metas ...internal.SymbolMeta) []*SearchResult {
		var results []*SearchResult
		for _, sm := range metas {
			results = append(results,
				&SearchResult{
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
		want []*SearchResult
	}{
		{
			name: "test search by <symbol>",
			q:    sample.Variable.Name,
			want: checkResult(sample.Variable.SymbolMeta),
		},
		/*
			TODO(golang/go#44142): Add support for searching just by field or
			method name.
			{
				name: "test search by <methodName>",
				q:    "Method",
				want: checkResult(sample.Method),
			},
		*/
		{
			name: "test search by <package>.<type>.<methodName>",
			q:    "foo.Type.Method",
			want: checkResult(sample.Method),
		},
		{
			name: "test search by <package> dot <identifier>",
			q:    "foo.Variable",
			want: checkResult(sample.Variable.SymbolMeta),
		},
		{
			name: "test search by <type>.<field>",
			q:    "Type.Field",
			want: checkResult(sample.Field),
		},
		{
			name: "test search by <type>.<method>",
			q:    "Type.Method",
			want: checkResult(sample.Method),
		},
		{
			name: "test search by <package> space <identifier>",
			q:    "foo function",
			want: checkResult(sample.Function.SymbolMeta),
		},
		{
			name: "test search by <package-subpath> <package-name> <identifier>",
			q:    "github.com/valid foo function",
			want: checkResult(sample.Function.SymbolMeta),
		},
		{
			name: "test search by <package-subpath> <identifier> subpath contains _",
			q:    "module_name/foo function",
			want: checkResult(sample.Function.SymbolMeta),
		},
		{
			name: "test invalid to_tsquery input returns no results instead of error",
			q:    "foo:function",
		},
	} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(test.name, "<", "_"), ">", "_"), func(t *testing.T) {
			opts := SearchOptions{
				Offset:         0,
				MaxResultCount: 100,
			}
			resp, err := testDB.hedgedSearch(ctx, test.q, 2, opts, symbolSearchers, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.results) == 0 && test.want != nil {
				t.Fatalf("expected results")
			}
			if diff := cmp.Diff(test.want, resp.results); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestUpsertSymbolSearch_UniqueConstraints tests for this upsert error:
// ERROR: ON CONFLICT DO UPDATE command cannot affect row a second time
// (SQLSTATE 21000)
func TestUpsertSymbolSearch_UniqueConstraints(t *testing.T) {
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	m := sample.DefaultModule()
	m.Packages()[0].Documentation[0].API = sample.API
	MustInsertModule(ctx, t, testDB, m)

	m2 := sample.DefaultModule()
	m2.Version = "v1.1.0"
	m2.Packages()[0].Documentation[0].API = sample.API
	MustInsertModule(ctx, t, testDB, m2)
}

func TestMultiwordSearchCombinations(t *testing.T) {
	for _, test := range []struct {
		q, filter string
		want      map[string]string
	}{
		{
			q: "github.com foo",
			want: map[string]string{
				"foo": "github.com",
			},
		},
		{
			q: "julieqiu foo",
			want: map[string]string{
				"julieqiu": "foo",
				"foo":      "julieqiu",
			},
		},
		{
			q: "github.com/julieqiu foo",
			want: map[string]string{
				"foo": "github.com/julieqiu",
			},
		},
		{
			q: "github.com julieqiu/api-demo foo",
			want: map[string]string{
				"foo": "github.com & julieqiu/api-demo",
			},
		},
		{
			q:    "github.com julieqiu/api-demo",
			want: nil,
		},
		{
			q: "bee cmd command",
			want: map[string]string{
				"bee":     "cmd & command",
				"cmd":     "bee & command",
				"command": "bee & cmd",
			},
		},
		{
			q:    "bee beego cmd command",
			want: nil,
		},
		{
			q:      "bee beego cmd command",
			filter: "command",
			want: map[string]string{
				"command": "bee & beego & cmd",
			},
		},
	} {
		t.Run(test.q, func(t *testing.T) {
			got := multiwordSearchCombinations(test.q, test.filter)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
