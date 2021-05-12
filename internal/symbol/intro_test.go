// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
)

func TestIntroducedHistory_OneBuildContext(t *testing.T) {
	input := internal.NewSymbolHistory()
	for _, s := range []struct {
		name, version string
	}{
		{"Foo", "v1.0.0"},
		{"Foo", "v1.2.0"},
		{"Foo.A", "v1.2.0"},
		{"Bar", "v1.1.0"},
	} {
		sm := internal.SymbolMeta{Name: s.name}
		for _, b := range internal.BuildContexts {
			input.AddSymbol(sm, s.version, b)
		}
	}
	want := internal.NewSymbolHistory()
	want.AddSymbol(
		internal.SymbolMeta{Name: "Foo"},
		"v1.0.0",
		internal.BuildContextAll,
	)
	want.AddSymbol(
		internal.SymbolMeta{Name: "Bar"},
		"v1.1.0",
		internal.BuildContextAll,
	)
	want.AddSymbol(
		internal.SymbolMeta{Name: "Foo.A"},
		"v1.2.0",
		internal.BuildContextAll,
	)
	got, err := IntroducedHistory(input)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{}),
		cmpopts.IgnoreFields(internal.SymbolBuildContexts{}, "builds")); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
	for _, v := range got.Versions() {
		nts := got.SymbolsAtVersion(v)
		for _, stu := range nts {
			for _, g := range stu {
				if !g.InAll() {
					t.Errorf("got build contexts = %v; want all", g.BuildContexts())
				}
			}
		}
	}
}

func TestIntroducedHistory_MultiGOOS(t *testing.T) {
	input := internal.NewSymbolHistory()
	for _, s := range []struct {
		name, version string
		build         internal.BuildContext
	}{
		{"Bar", "v1.0.0", internal.BuildContextWindows},
		{"Bar", "v1.0.0", internal.BuildContextLinux},
		{"Bar", "v1.0.0", internal.BuildContextJS},
		{"Bar", "v1.0.0", internal.BuildContextDarwin},
		{"Foo", "v1.0.0", internal.BuildContextWindows},
		{"Foo", "v1.0.0", internal.BuildContextLinux},
		{"Foo", "v1.1.0", internal.BuildContextLinux},
		{"Foo.A", "v1.1.0", internal.BuildContextLinux},
		{"Foo", "v1.1.0", internal.BuildContextJS},
		{"Foo.A", "v1.2.0", internal.BuildContextJS},
	} {
		sm := internal.SymbolMeta{Name: s.name}
		input.AddSymbol(sm, s.version, s.build)
	}

	want := internal.NewSymbolHistory()
	withSym := func(name, v string, builds []internal.BuildContext) {
		s := internal.SymbolMeta{Name: name}
		for _, b := range builds {
			want.AddSymbol(s, v, b)
		}
	}
	for _, s := range []struct {
		n, v   string
		builds []internal.BuildContext
	}{
		{"Bar", "v1.0.0", []internal.BuildContext{internal.BuildContextAll}},
		{"Foo", "v1.0.0", []internal.BuildContext{internal.BuildContextLinux, internal.BuildContextWindows}},
		{"Foo", "v1.1.0", []internal.BuildContext{internal.BuildContextJS}},
		{"Foo.A", "v1.1.0", []internal.BuildContext{internal.BuildContextLinux}},
		{"Foo.A", "v1.2.0", []internal.BuildContext{internal.BuildContextJS}},
	} {
		withSym(s.n, s.v, s.builds)
	}

	got, err := IntroducedHistory(input)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got,
		cmp.AllowUnexported(internal.SymbolBuildContexts{}, internal.SymbolHistory{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
