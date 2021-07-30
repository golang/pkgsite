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
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestInsertSymbolNamesAndHistory(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
	)
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

	got, err := testDB.db.CollectStrings(ctx, `SELECT name FROM symbol_names;`)
	if err != nil {
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
		map[internal.BuildContext][]*internal.Symbol{internal.BuildContextAll: api})
	want2 := symbolHistoryFromAPI(api, mod.Version)
	comparePackageSymbols(ctx, t, testDB, mod.Packages()[0].Path, mod.ModulePath, mod.Version, want2)

	gotHist, err := testDB.GetSymbolHistory(ctx, mod.Packages()[0].Path, mod.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	wantHist := internal.NewSymbolHistory()
	wantHist.AddSymbol(sample.Constant.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(sample.Variable.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(sample.Function.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(sample.Type.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(*sample.Type.Children[0], "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(*sample.Type.Children[1], "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(*sample.Type.Children[2], "v1.0.0", internal.BuildContextAll)
	if diff := cmp.Diff(wantHist, gotHist,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{})); diff != "" {
		t.Fatalf("mismatch on symbol history(-want +got):\n%s", diff)
	}
}

func TestInsertSymbolHistory_Basic(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
	)
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

	compareUnitSymbols(ctx, t, testDB, mod.Packages()[0].Path, mod.ModulePath, mod.Version,
		map[internal.BuildContext][]*internal.Symbol{internal.BuildContextAll: api})
	want2 := symbolHistoryFromAPI(api, mod.Version)
	comparePackageSymbols(ctx, t, testDB, mod.Packages()[0].Path, mod.ModulePath, mod.Version, want2)
}

func TestInsertSymbolHistory_MultiVersions(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
	)
	defer cancel()

	typ := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo",
			Synopsis:   "type Foo struct",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindType,
			ParentName: "Foo",
		},
	}
	methodA := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo.A",
			Synopsis:   "func (*Foo) A()",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindMethod,
			ParentName: typ.Name,
		},
	}
	methodB := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo.B",
			Synopsis:   "func (*Foo) B()",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindMethod,
			ParentName: typ.Name,
		},
	}
	typA := typ
	typA.Children = []*internal.SymbolMeta{&methodA.SymbolMeta}
	typB := typ
	typB.Children = []*internal.SymbolMeta{&methodA.SymbolMeta, &methodB.SymbolMeta}

	mod10 := moduleWithSymbols(t, "v1.0.0", []*internal.Symbol{&typ})
	mod11 := moduleWithSymbols(t, "v1.1.0", []*internal.Symbol{&typA})
	mod12 := moduleWithSymbols(t, "v1.2.0", []*internal.Symbol{&typB})

	// Insert most recent, then oldest, then middle version.
	MustInsertModule(ctx, t, testDB, mod12)
	MustInsertModule(ctx, t, testDB, mod10)
	MustInsertModule(ctx, t, testDB, mod11)

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

	want2 := internal.NewSymbolHistory()
	for _, want := range []struct {
		version        string
		buildToSymbols map[internal.BuildContext][]*internal.Symbol
	}{
		{mod10.Version, want10},
		{mod11.Version, want11},
		{mod12.Version, want12},
	} {
		for build, api := range want.buildToSymbols {
			updateSymbols(api, func(s *internal.SymbolMeta) error {
				want2.AddSymbol(*s, want.version, build)
				return nil
			})
		}
	}
	comparePackageSymbols(ctx, t, testDB, mod10.Packages()[0].Path, mod10.ModulePath, mod10.Version, want2)

	gotHist, err := testDB.GetSymbolHistory(ctx, mod10.Packages()[0].Path, mod10.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	typA.GOOS = internal.All
	methodA.GOOS = internal.All
	methodB.GOOS = internal.All
	typA.GOARCH = internal.All
	methodA.GOARCH = internal.All
	methodB.GOARCH = internal.All
	wantHist := internal.NewSymbolHistory()
	wantHist.AddSymbol(typA.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(methodA.SymbolMeta, "v1.1.0", internal.BuildContextAll)
	wantHist.AddSymbol(methodB.SymbolMeta, "v1.2.0", internal.BuildContextAll)
	if diff := cmp.Diff(wantHist, gotHist,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{})); diff != "" {
		t.Fatalf("mismatch on symbol history(-want +got):\n%s", diff)
	}
}

func TestInsertSymbolHistory_MultiGOOS(t *testing.T) {
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
	)
	defer cancel()

	typ := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo",
			Synopsis:   "type Foo struct",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindType,
			ParentName: "Foo",
		},
	}
	methodA := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo.A",
			Synopsis:   "func (*Foo) A()",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindMethod,
			ParentName: typ.Name,
		},
	}
	methodB := internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "Foo.B",
			Synopsis:   "func (*Foo) B()",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindMethod,
			ParentName: typ.Name,
		},
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
			newTyp.Children = append(newTyp.Children, &m.SymbolMeta)
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

	want2 := internal.NewSymbolHistory()
	for _, mod := range []*internal.Module{mod10, mod11, mod12} {
		for _, pkg := range mod.Packages() {
			for _, doc := range pkg.Documentation {
				sh := symbolHistoryFromAPI(doc.API, mod.Version)
				for _, v := range sh.Versions() {
					nts := sh.SymbolsAtVersion(v)
					for _, stu := range nts {
						for sm, us := range stu {
							for _, b := range us.BuildContexts() {
								want2.AddSymbol(sm, mod.Version, b)
							}
						}
					}
				}
			}
		}
	}
	comparePackageSymbols(ctx, t, testDB, mod10.Packages()[0].Path, mod10.ModulePath, mod10.Version, want2)

	gotHist, err := testDB.GetSymbolHistory(ctx, mod10.Packages()[0].Path, mod10.ModulePath)
	if err != nil {
		t.Fatal(err)
	}

	typ.GOOS = internal.All
	typ.GOARCH = internal.All
	wantHist := internal.NewSymbolHistory()
	wantHist.AddSymbol(typ.SymbolMeta, "v1.0.0", internal.BuildContextAll)
	wantHist.AddSymbol(methodA.SymbolMeta, "v1.1.0", internal.BuildContextLinux)
	wantHist.AddSymbol(methodA.SymbolMeta, "v1.1.0", internal.BuildContextWindows)
	wantHist.AddSymbol(methodB.SymbolMeta, "v1.1.0", internal.BuildContextJS)
	wantHist.AddSymbol(methodB.SymbolMeta, "v1.1.0", internal.BuildContextDarwin)
	wantHist.AddSymbol(methodA.SymbolMeta, "v1.2.0", internal.BuildContextJS)
	wantHist.AddSymbol(methodA.SymbolMeta, "v1.2.0", internal.BuildContextDarwin)
	wantHist.AddSymbol(methodB.SymbolMeta, "v1.2.0", internal.BuildContextLinux)
	wantHist.AddSymbol(methodB.SymbolMeta, "v1.2.0", internal.BuildContextWindows)
	if diff := cmp.Diff(wantHist, gotHist,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{})); diff != "" {
		t.Fatalf("mismatch on symbol history(-want +got):\n%s", diff)
	}

	pathID, err := GetPathID(ctx, testDB.db, mod10.Packages()[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	gotHist2, err := GetSymbolHistoryForBuildContext(ctx, testDB.db,
		pathID, mod10.ModulePath, internal.BuildContextWindows)
	if err != nil {
		t.Fatal(err)
	}
	wantHist2 := map[string]string{
		"Foo":   "v1.0.0",
		"Foo.A": "v1.1.0",
		"Foo.B": "v1.2.0",
	}
	if diff := cmp.Diff(wantHist2, gotHist2); diff != "" {
		t.Fatalf("mismatch on symbol history(-want +got):\n%s", diff)
	}
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
		if diff := cmp.Diff(want, got,
			cmpopts.IgnoreFields(internal.Symbol{}, "GOOS", "GOARCH")); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func comparePackageSymbols(ctx context.Context, t *testing.T, testDB *DB,
	path, modulePath, version string, want *internal.SymbolHistory) {
	t.Helper()
	got, err := getPackageSymbols(ctx, testDB.db, path, modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{}),
		cmpopts.IgnoreFields(internal.SymbolBuildContexts{}, "builds")); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func symbolHistoryFromAPI(api []*internal.Symbol, version string) *internal.SymbolHistory {
	sh := internal.NewSymbolHistory()
	updateSymbols(api, func(s *internal.SymbolMeta) error {
		sh.AddSymbol(*s, version, internal.BuildContextAll)
		return nil
	})
	return sh
}

// getUnitSymbols returns all of the symbols for the given unitID.
func getUnitSymbols(ctx context.Context, db *database.DB, unitID int) (_ map[internal.BuildContext][]*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "getUnitSymbols(ctx, db, %d)", unitID)

	// Fetch all symbols for the unit. Order by symbol_type "Type" first, so
	// that when we collect the children the structs for these symbols will
	// already be created.
	query := `
        SELECT
            s1.name AS symbol_name,
            s2.name AS parent_symbol_name,
            ps.section,
            ps.type,
            ps.synopsis,
            d.goos,
            d.goarch
        FROM documentation_symbols ds
        INNER JOIN documentation d ON d.id = ds.documentation_id
        INNER JOIN package_symbols ps ON ds.package_symbol_id = ps.id
        INNER JOIN symbol_names s1 ON ps.symbol_name_id = s1.id
        INNER JOIN symbol_names s2 ON ps.parent_symbol_name_id = s2.id
        WHERE d.unit_id = $1
        ORDER BY CASE WHEN ps.type='Type' THEN 0 ELSE 1 END;`
	// buildToSymbols contains all of the symbols for this unit, grouped by
	// build context.
	buildToSymbols := map[internal.BuildContext][]*internal.Symbol{}
	// buildToNameToType contains all of the types for this unit, grouped by
	// name and build context. This is used to keep track of the parent types,
	// so that we can map the children to those symbols.
	buildToNameToType := map[internal.BuildContext]map[string]*internal.Symbol{}
	collect := func(rows *sql.Rows) error {
		var (
			sm    internal.SymbolMeta
			build internal.BuildContext
		)
		if err := rows.Scan(
			&sm.Name, &sm.ParentName,
			&sm.Section, &sm.Kind, &sm.Synopsis,
			&build.GOOS, &build.GOARCH); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}

		s := &internal.Symbol{
			SymbolMeta: sm,
			GOOS:       build.GOOS,
			GOARCH:     build.GOARCH,
		}
		switch sm.Section {
		// For symbols that belong to a type, map that symbol as a children of
		// the parent type.
		case internal.SymbolSectionTypes:
			if sm.Kind == internal.SymbolKindType {
				_, ok := buildToNameToType[build]
				if !ok {
					buildToNameToType[build] = map[string]*internal.Symbol{}
				}
				buildToNameToType[build][sm.Name] = s
				buildToSymbols[build] = append(buildToSymbols[build], s)
			} else {
				nameToType, ok := buildToNameToType[build]
				if !ok {
					return fmt.Errorf("build context %v for parent type %q could not be found for symbol %q", build, sm.ParentName, sm.Name)
				}
				parent, ok := nameToType[sm.ParentName]
				if !ok {
					return fmt.Errorf("parent type %q could not be found for symbol %q", sm.ParentName, sm.Name)
				}
				parent.Children = append(parent.Children, &sm)
			}
		default:
			buildToSymbols[build] = append(buildToSymbols[build], s)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, unitID); err != nil {
		return nil, err
	}
	return buildToSymbols, nil
}
