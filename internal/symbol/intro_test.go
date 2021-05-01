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
	input := map[string]map[string]*internal.UnitSymbol{}
	for _, s := range []struct {
		name, version string
	}{
		{"Foo", "v1.0.0"},
		{"Foo", "v1.2.0"},
		{"Foo.A", "v1.2.0"},
		{"Bar", "v1.1.0"},
	} {
		if _, ok := input[s.version]; !ok {
			input[s.version] = map[string]*internal.UnitSymbol{}
		}
		us := &internal.UnitSymbol{
			SymbolMeta: internal.SymbolMeta{
				Name: s.name,
			},
		}
		for _, b := range internal.BuildContexts {
			us.AddBuildContext(b)
		}
		input[s.version][s.name] = us
	}
	want := map[string]map[string]*internal.UnitSymbol{
		"v1.0.0": {
			"Foo": &internal.UnitSymbol{
				SymbolMeta: internal.SymbolMeta{
					Name: "Foo",
				},
			},
		},
		"v1.1.0": {
			"Bar": &internal.UnitSymbol{
				SymbolMeta: internal.SymbolMeta{
					Name: "Bar",
				},
			},
		},
		"v1.2.0": {
			"Foo.A": &internal.UnitSymbol{
				SymbolMeta: internal.SymbolMeta{
					Name: "Foo.A",
				},
			},
		},
	}
	got := IntroducedHistory(input)
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.UnitSymbol{}, "builds")); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
	for _, nts := range got {
		for _, g := range nts {
			if !g.InAll() {
				t.Errorf("got build contexts = %v; want all", g.BuildContexts())
			}
		}
	}
}

func TestIntroducedHistory_MultiGOOS(t *testing.T) {
	input := map[string]map[string]*internal.UnitSymbol{}
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
		if _, ok := input[s.version]; !ok {
			input[s.version] = map[string]*internal.UnitSymbol{}
		}
		us, ok := input[s.version][s.name]
		if !ok {
			us = &internal.UnitSymbol{
				SymbolMeta: internal.SymbolMeta{
					Name: s.name,
				},
			}
			input[s.version][s.name] = us
		}
		us.AddBuildContext(s.build)
	}

	withBuilds := func(name string, builds ...internal.BuildContext) *internal.UnitSymbol {
		us := &internal.UnitSymbol{
			SymbolMeta: internal.SymbolMeta{
				Name: name,
			},
		}
		for _, b := range builds {
			us.AddBuildContext(b)
		}
		return us
	}
	want := map[string]map[string]*internal.UnitSymbol{
		"v1.0.0": {
			"Bar": withBuilds("Bar", internal.BuildContextAll),
			"Foo": withBuilds("Foo", internal.BuildContextLinux, internal.BuildContextWindows),
		},
		"v1.1.0": {
			"Foo":   withBuilds("Foo", internal.BuildContextJS),
			"Foo.A": withBuilds("Foo.A", internal.BuildContextLinux),
		},
		"v1.2.0": {
			"Foo.A": withBuilds("Foo.A", internal.BuildContextJS),
		},
	}
	got := IntroducedHistory(input)
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.UnitSymbol{}, "builds")); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
	for v, nts := range got {
		for n, g := range nts {
			w := want[v][n]
			if diff := cmp.Diff(w.BuildContexts(), g.BuildContexts()); diff != "" {
				t.Errorf("(%s %s): got build contexts = %v; want %v", n, v, g.BuildContexts(), w.BuildContexts())
			}
		}
	}
}
