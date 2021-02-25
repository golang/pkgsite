// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestShouldUpdateSymbolHistory(t *testing.T) {
	testSym := "Foo"
	for _, test := range []struct {
		name       string
		newVersion string
		oldHist    map[string]*internal.Symbol
		want       bool
	}{
		{
			name:    "should update when new version is older",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: "v1.2.3"}},
			want:    true,
		},
		{
			name:    "should update when symbol does not exist",
			oldHist: map[string]*internal.Symbol{},
			want:    true,
		},
		{
			name:    "should update when new version is the same",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: sample.VersionString}},
			want:    true,
		},
		{
			name:    "should not update when new version is newer",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: "v0.1.0"}},
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldUpdateSymbolHistory(testSym, sample.VersionString, test.oldHist); got != test.want {
				t.Errorf("shouldUpdateSymbolHistory(%q, %q, %+v) = %t; want = %t",
					testSym, sample.VersionString, test.oldHist, got, test.want)
			}
		})
	}
}

func TestInsertSymbolNamesAndHistory(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbols, internal.ExperimentInsertSymbolHistory)
	defer cancel()

	mod := sample.DefaultModule()
	if len(mod.Packages()) != 1 {
		t.Fatalf("len(mod.Packages()) = %d; want 1", len(mod.Packages()))
	}
	if len(mod.Packages()[0].Documentation) != 1 {
		t.Fatalf("len(mod.Packages()[0].Documentation) = %d; want 1", len(mod.Packages()[0].Documentation))
	}
	api := []*internal.Symbol{
		sample.Constant,
		sample.Variable,
		sample.Function,
		sample.Type,
	}
	mod.Packages()[0].Documentation[0].API = api
	MustInsertModule(ctx, t, testDB, mod)

	var got []string
	if err := testDB.db.RunQuery(ctx, `SELECT name FROM symbol_names;`, func(rows *sql.Rows) error {
		var n string
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		got = append(got, n)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		sample.Constant.Name,
		sample.Variable.Name,
		sample.Function.Name,
		sample.Type.Name,
	}
	for _, c := range sample.Type.Children {
		want = append(want, c.Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	compareUnitSymbols(ctx, t, testDB, mod.Packages()[0].Path, mod.ModulePath, mod.Version,
		map[internal.BuildContext][]*internal.Symbol{
			internal.BuildContextAll: api,
		})
}

func TestInsertSymbolHistory_Basic(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbols, internal.ExperimentInsertSymbolHistory)
	defer cancel()

	mod := sample.DefaultModule()
	if len(mod.Packages()) != 1 {
		t.Fatalf("len(mod.Packages()) = %d; want 1", len(mod.Packages()))
	}
	if len(mod.Packages()[0].Documentation) != 1 {
		t.Fatalf("len(mod.Packages()[0].Documentation) = %d; want 1", len(mod.Packages()[0].Documentation))
	}
	api := []*internal.Symbol{
		sample.Constant,
		sample.Variable,
		sample.Function,
		sample.Type,
	}
	mod.Packages()[0].Documentation[0].API = api
	MustInsertModule(ctx, t, testDB, mod)

	gotHist, err := getSymbolHistory(ctx, testDB.db, mod.Packages()[0].Path, mod.ModulePath)
	if err != nil {
		t.Fatal(err)
	}

	symbols := map[string]*internal.Symbol{
		"Constant": {
			Name:         "Constant",
			Synopsis:     "const Constant",
			Section:      "Constants",
			Kind:         "Constant",
			ParentName:   "Constant",
			SinceVersion: "v1.0.0",
		},
		"Function": {
			Name:         "Function",
			Synopsis:     "func Function() error",
			Section:      "Functions",
			Kind:         "Function",
			ParentName:   "Function",
			SinceVersion: "v1.0.0",
		},
		"Type": {
			Name:         "Type",
			Synopsis:     "type Type struct",
			Section:      "Types",
			Kind:         "Type",
			ParentName:   "Type",
			SinceVersion: "v1.0.0",
		},
		"Variable": {
			Name:         "Variable",
			Synopsis:     "var Variable",
			Section:      "Variables",
			Kind:         "Variable",
			ParentName:   "Variable",
			SinceVersion: "v1.0.0",
		},
		"Type.Field": {
			Name:         "Type.Field",
			Synopsis:     "field",
			Section:      "Types",
			Kind:         "Field",
			ParentName:   "Type",
			SinceVersion: "v1.0.0",
		},
		"Type.Method": {
			Name:         "Type.Method",
			Synopsis:     "method",
			Section:      "Types",
			Kind:         "Method",
			ParentName:   "Type",
			SinceVersion: "v1.0.0",
		},
		"New": {
			Name:         "New",
			Synopsis:     "func New() *Type",
			Section:      "Types",
			Kind:         "Function",
			ParentName:   "Type",
			SinceVersion: "v1.0.0",
		},
	}
	wantHist := map[internal.BuildContext]map[string]*internal.Symbol{
		internal.BuildContextDarwin:  symbols,
		internal.BuildContextJS:      symbols,
		internal.BuildContextLinux:   symbols,
		internal.BuildContextWindows: symbols,
	}
	if diff := cmp.Diff(wantHist, gotHist,
		cmpopts.IgnoreFields(internal.Symbol{}, "GOOS", "GOARCH")); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
	compareUnitSymbols(ctx, t, testDB, mod.Packages()[0].Path, mod.ModulePath, mod.Version,
		map[internal.BuildContext][]*internal.Symbol{
			internal.BuildContextAll: api,
		})
}

func TestInsertSymbolHistory_MultiVersions(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbols, internal.ExperimentInsertSymbolHistory)
	defer cancel()

	typ := internal.Symbol{
		Name:         "Foo",
		Synopsis:     "type Foo struct",
		Section:      internal.SymbolSectionTypes,
		Kind:         internal.SymbolKindType,
		ParentName:   "Foo",
		SinceVersion: "v1.0.0",
	}
	methodA := internal.Symbol{
		Name:         "Foo.A",
		Synopsis:     "func (*Foo) A()",
		Section:      internal.SymbolSectionTypes,
		Kind:         internal.SymbolKindMethod,
		ParentName:   typ.Name,
		SinceVersion: "v1.1.0",
	}
	methodB := internal.Symbol{
		Name:         "Foo.B",
		Synopsis:     "func (*Foo) B()",
		Section:      internal.SymbolSectionTypes,
		Kind:         internal.SymbolKindMethod,
		ParentName:   typ.Name,
		SinceVersion: "v1.2.0",
	}
	typA := typ
	typA.Children = []*internal.Symbol{&methodA}
	typB := typ
	typB.Children = []*internal.Symbol{&methodA, &methodB}

	mod10 := moduleWithSymbols(t, "v1.0.0", []*internal.Symbol{&typ})
	mod11 := moduleWithSymbols(t, "v1.1.0", []*internal.Symbol{&typA})
	mod12 := moduleWithSymbols(t, "v1.2.0", []*internal.Symbol{&typB})

	// Insert most recent, then oldest, then middle version.
	MustInsertModule(ctx, t, testDB, mod12)
	MustInsertModule(ctx, t, testDB, mod10)
	MustInsertModule(ctx, t, testDB, mod11)

	symbols := map[string]*internal.Symbol{
		"Foo":   &typ,
		"Foo.A": &methodA,
		"Foo.B": &methodB,
	}
	wantHist := map[internal.BuildContext]map[string]*internal.Symbol{
		internal.BuildContextDarwin:  symbols,
		internal.BuildContextJS:      symbols,
		internal.BuildContextLinux:   symbols,
		internal.BuildContextWindows: symbols,
	}
	gotHist, err := getSymbolHistory(ctx, testDB.db, mod12.Packages()[0].Path, mod12.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantHist, gotHist,
		cmpopts.IgnoreFields(internal.Symbol{}, "GOOS", "GOARCH")); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
	createwant := func(docs []*internal.Documentation) map[internal.BuildContext][]*internal.Symbol {
		want := map[internal.BuildContext][]*internal.Symbol{}
		for _, doc := range docs {
			want[internal.BuildContext{GOOS: doc.GOOS, GOARCH: doc.GOARCH}] = doc.API
		}
		return want
	}

	want10 := createwant(mod10.Packages()[0].Documentation)
	want11 := createwant(mod11.Packages()[0].Documentation)
	want12 := createwant(mod12.Packages()[0].Documentation)
	compareUnitSymbols(ctx, t, testDB, mod10.Packages()[0].Path, mod10.ModulePath, mod10.Version, want10)
	compareUnitSymbols(ctx, t, testDB, mod11.Packages()[0].Path, mod11.ModulePath, mod11.Version, want11)
	compareUnitSymbols(ctx, t, testDB, mod12.Packages()[0].Path, mod12.ModulePath, mod12.Version, want12)
}

func TestInsertSymbolHistory_MultiGOOS(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbols, internal.ExperimentInsertSymbolHistory)
	defer cancel()

	typ := internal.Symbol{
		Name:       "Foo",
		Synopsis:   "type Foo struct",
		Section:    internal.SymbolSectionTypes,
		Kind:       internal.SymbolKindType,
		ParentName: "Foo",
	}
	methodA := internal.Symbol{
		Name:       "Foo.A",
		Synopsis:   "func (*Foo) A()",
		Section:    internal.SymbolSectionTypes,
		Kind:       internal.SymbolKindMethod,
		ParentName: typ.Name,
	}
	methodB := internal.Symbol{
		Name:       "Foo.B",
		Synopsis:   "func (*Foo) B()",
		Section:    internal.SymbolSectionTypes,
		Kind:       internal.SymbolKindMethod,
		ParentName: typ.Name,
	}
	mod10 := moduleWithSymbols(t, "v1.0.0", []*internal.Symbol{&typ})
	mod11 := moduleWithSymbols(t, "v1.1.0", nil)
	makeDocs := func() []*internal.Documentation {
		return []*internal.Documentation{
			sample.Documentation(
				internal.BuildContextLinux.GOOS,
				internal.BuildContextLinux.GOARCH,
				sample.DocContents),
			sample.Documentation(
				internal.BuildContextWindows.GOOS,
				internal.BuildContextWindows.GOARCH,
				sample.DocContents),
			sample.Documentation(
				internal.BuildContextDarwin.GOOS,
				internal.BuildContextDarwin.GOARCH,
				sample.DocContents),
			sample.Documentation(
				internal.BuildContextJS.GOOS,
				internal.BuildContextJS.GOARCH,
				sample.DocContents),
		}
	}
	mod11.Packages()[0].Documentation = makeDocs()
	docs1 := mod11.Packages()[0].Documentation

	symsA := []internal.Symbol{methodA}
	symsB := []internal.Symbol{methodB}
	createType := func(methods []internal.Symbol, goos, goarch string) []*internal.Symbol {
		newTyp := typ
		newTyp.GOOS = goos
		newTyp.GOARCH = goarch

		for _, m := range methods {
			m.GOOS = goos
			m.GOARCH = goarch
			newTyp.Children = append(newTyp.Children, &m)
		}
		return []*internal.Symbol{&newTyp}
	}
	docs1[0].API = createType(symsA, docs1[0].GOOS, docs1[0].GOARCH)
	docs1[1].API = createType(symsA, docs1[1].GOOS, docs1[1].GOARCH)
	docs1[2].API = createType(symsB, docs1[2].GOOS, docs1[2].GOARCH)
	docs1[3].API = createType(symsB, docs1[3].GOOS, docs1[3].GOARCH)
	mod11.Packages()[0].Documentation = docs1

	mod12 := moduleWithSymbols(t, "v1.2.0", nil)
	mod12.Packages()[0].Documentation = makeDocs()
	docs2 := mod12.Packages()[0].Documentation
	docs2[0].API = createType(symsB, docs2[0].GOOS, docs2[0].GOARCH)
	docs2[1].API = createType(symsB, docs2[1].GOOS, docs2[1].GOARCH)
	docs2[2].API = createType(symsA, docs2[2].GOOS, docs2[2].GOARCH)
	docs2[3].API = createType(symsA, docs2[3].GOOS, docs2[3].GOARCH)
	mod12.Packages()[0].Documentation = docs2

	// Insert most recent, then oldest, then middle version.
	MustInsertModule(ctx, t, testDB, mod12)
	MustInsertModule(ctx, t, testDB, mod10)
	MustInsertModule(ctx, t, testDB, mod11)
	gotHist, err := getSymbolHistory(ctx, testDB.db, mod10.Packages()[0].Path, mod10.ModulePath)
	if err != nil {
		t.Fatal(err)
	}

	parent := func() *internal.Symbol { typ.SinceVersion = "v1.0.0"; return &typ }()
	a1 := func() *internal.Symbol { a := methodA; a.SinceVersion = "v1.1.0"; return &a }()
	b1 := func() *internal.Symbol { b := methodB; b.SinceVersion = "v1.2.0"; return &b }()
	a2 := func() *internal.Symbol { a := methodA; a.SinceVersion = "v1.2.0"; return &a }()
	b2 := func() *internal.Symbol { b := methodB; b.SinceVersion = "v1.1.0"; return &b }()
	wantHist := map[internal.BuildContext]map[string]*internal.Symbol{
		internal.BuildContextLinux: {
			"Foo":   parent,
			"Foo.A": a1,
			"Foo.B": b1,
		},
		internal.BuildContextWindows: {
			"Foo":   parent,
			"Foo.A": a1,
			"Foo.B": b1,
		},
		internal.BuildContextDarwin: {
			"Foo":   parent,
			"Foo.A": a2,
			"Foo.B": b2,
		},
		internal.BuildContextJS: {
			"Foo":   parent,
			"Foo.A": a2,
			"Foo.B": b2,
		},
	}
	if diff := cmp.Diff(wantHist, gotHist,
		cmpopts.IgnoreFields(internal.Symbol{}, "GOOS", "GOARCH")); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	createwant := func(docs []*internal.Documentation) map[internal.BuildContext][]*internal.Symbol {
		want := map[internal.BuildContext][]*internal.Symbol{}
		for _, doc := range docs {
			want[internal.BuildContext{GOOS: doc.GOOS, GOARCH: doc.GOARCH}] = doc.API
		}
		return want
	}
	want10 := createwant(mod10.Packages()[0].Documentation)
	want11 := createwant(mod11.Packages()[0].Documentation)
	want12 := createwant(mod12.Packages()[0].Documentation)
	compareUnitSymbols(ctx, t, testDB, mod10.Packages()[0].Path, mod10.ModulePath, mod10.Version, want10)
	compareUnitSymbols(ctx, t, testDB, mod11.Packages()[0].Path, mod11.ModulePath, mod11.Version, want11)
	compareUnitSymbols(ctx, t, testDB, mod12.Packages()[0].Path, mod12.ModulePath, mod12.Version, want12)
}

func moduleWithSymbols(t *testing.T, version string, symbols []*internal.Symbol) *internal.Module {
	mod := sample.Module(sample.ModulePath, version, "")
	if len(mod.Packages()) != 1 {
		t.Fatalf("len(mod.Packages()) = %d; want 1", len(mod.Packages()))
	}
	if len(mod.Packages()[0].Documentation) != 1 {
		t.Fatalf("len(mod.Packages()[0].Documentation) = %d; want 1", len(mod.Packages()[0].Documentation))
	}
	// symbols for goos/goarch = all/all
	mod.Packages()[0].Documentation[0].API = symbols
	return mod
}

func compareUnitSymbols(ctx context.Context, t *testing.T, testDB *DB,
	path, modulePath, version string, wantBuildToSymbols map[internal.BuildContext][]*internal.Symbol) {
	t.Helper()
	unitID, err := testDB.getUnitID(ctx, path, modulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	buildToSymbols, err := getUnitSymbols(ctx, testDB.db, unitID)
	if err != nil {
		t.Fatal(err)
	}
	for build, got := range buildToSymbols {
		want := wantBuildToSymbols[build]
		sort.Slice(got, func(i, j int) bool {
			return got[i].Synopsis < got[j].Synopsis
		})
		for _, s := range got {
			sort.Slice(s.Children, func(i, j int) bool {
				return s.Children[i].Synopsis < s.Children[j].Synopsis
			})
		}
		sort.Slice(want, func(i, j int) bool {
			return want[i].Synopsis < want[j].Synopsis
		})
		for _, s := range want {
			sort.Slice(s.Children, func(i, j int) bool {
				return s.Children[i].Synopsis < s.Children[j].Synopsis
			})
		}
		if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.Symbol{}, "SinceVersion", "GOOS", "GOARCH")); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}
