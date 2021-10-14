// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

type testUnitPage struct {
	unit       *internal.UnitMeta
	name       string
	wantTitle  string
	wantType   string
	wantLabels []string
}

func TestPageTitlePageTypePageLabels(t *testing.T) {

	var tests []*testUnitPage
	m := sample.Module("golang.org/x/tools", "v1.0.0", "go/packages", "cmd/godoc")
	for _, u := range m.Units {
		um := &u.UnitMeta
		switch um.Path {
		case "golang.org/x/tools":
			tests = append(tests, &testUnitPage{um, "module golang.org/x/tools", "tools", pageTypeModule, []string{pageTypeModule}})
		case "golang.org/x/tools/go/packages":
			tests = append(tests, &testUnitPage{um, "package golang.org/x/tools/go/packages", "packages", pageTypePackage, []string{pageTypePackage}})
		case "golang.org/x/tools/go":
			tests = append(tests, &testUnitPage{um, "directory golang.org/x/tools/go", "go/", pageTypeDirectory, []string{pageTypeDirectory}})
		case "golang.org/x/tools/cmd/godoc":
			um.Name = "main"
			tests = append(tests, &testUnitPage{um, "package golang.org/x/tools/cmd/godoc", "godoc", pageTypeCommand, []string{pageTypeCommand}})
		case "golang.org/x/tools/cmd":
			tests = append(tests, &testUnitPage{um, "directory golang.org/x/tools/cmd", "cmd/", pageTypeDirectory, []string{pageTypeDirectory}})
		default:
			t.Fatalf("Unexpected path: %q", um.Path)
		}
	}

	m2 := sample.Module("golang.org/x/tools/gopls", "v1.0.0", "")
	m2.Units[0].Name = "main"
	tests = append(tests, &testUnitPage{&m2.Units[0].UnitMeta, "module golang.org/x/tools/gopls", "gopls", pageTypeCommand, []string{pageTypeCommand, pageTypeModule}})

	m3 := sample.Module("mvdan.cc/sh/v3", "v3.0.0")
	tests = append(tests, &testUnitPage{&m3.Units[0].UnitMeta, "module mvdan.cc/sh/v3", "sh", pageTypeModule, []string{pageTypeModule}})

	std := sample.Module(stdlib.ModulePath, "v1.0.0", "cmd/go")
	for _, u := range std.Units {
		um := &u.UnitMeta
		switch um.Path {
		case stdlib.ModulePath:
			tests = append(tests, &testUnitPage{um, "module std", "Standard library", pageTypeModuleStd, nil})
		case "cmd":
			tests = append(tests, &testUnitPage{um, "directory cmd", "cmd/", pageTypeDirectory, []string{pageTypeDirectory, pageTypeStdlib}})
		case "cmd/go":
			um.Name = "main"
			tests = append(tests, &testUnitPage{um, "command go", "go", pageTypeCommand, []string{pageTypeCommand, pageTypeStdlib}})
		default:
			t.Fatalf("Unexpected path: %q", um.Path)
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotTitle := pageTitle(test.unit)
			if gotTitle != test.wantTitle {
				t.Errorf("pageTitle(%q): %q; want = %q", test.unit.Path, gotTitle, test.wantTitle)
			}
			gotType := pageType(test.unit)
			if gotType != test.wantType {
				t.Errorf("pageType(%q): %q; want = %q", test.unit.Path, gotType, test.wantType)
			}
			gotLabels := pageLabels(test.unit)
			if diff := cmp.Diff(test.wantLabels, gotLabels); diff != "" {
				t.Errorf("mismatch on pageLabels (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAbsoluteTime(t *testing.T) {
	now := sample.NowTruncated()
	testCases := []struct {
		name         string
		date         time.Time
		absoluteTime string
	}{
		{
			name:         "today",
			date:         now.Add(time.Hour),
			absoluteTime: now.Add(time.Hour).Format("Jan _2, 2006"),
		},
		{
			name:         "a_week_ago",
			date:         now.Add(time.Hour * 24 * -5),
			absoluteTime: now.Add(time.Hour * 24 * -5).Format("Jan _2, 2006"),
		},
		{
			name:         "zero time",
			date:         time.Time{},
			absoluteTime: "unknown",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			absoluteTime := absoluteTime(test.date)

			if absoluteTime != test.absoluteTime {
				t.Errorf("absoluteTime(%q) = %s, want %s", test.date, absoluteTime, test.absoluteTime)
			}
		})
	}
}
