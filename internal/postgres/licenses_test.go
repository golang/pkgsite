// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetLicenses(t *testing.T) {
	t.Parallel()
	testModule := sample.Module(sample.ModulePath, "v1.2.3", "A/B")
	stdlibModule := sample.Module(stdlib.ModulePath, "v1.13.0", "cmd/go")
	mit := &licenses.Metadata{Types: []string{"MIT"}, FilePath: "LICENSE"}
	bsd := &licenses.Metadata{Types: []string{"BSD-3-Clause"}, FilePath: "A/B/LICENSE"}

	mitLicense := &licenses.License{Metadata: mit}
	bsdLicense := &licenses.License{Metadata: bsd}
	testModule.Licenses = []*licenses.License{bsdLicense, mitLicense}
	sort.Slice(testModule.Units, func(i, j int) bool {
		return testModule.Units[i].Path < testModule.Units[j].Path
	})

	// github.com/valid/module_name
	testModule.Units[0].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A
	testModule.Units[1].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A/B
	testModule.Units[2].Licenses = []*licenses.Metadata{mit, bsd}

	testDB, release := acquire(t)
	defer release()

	ctx := context.Background()
	MustInsertModule(ctx, t, testDB, testModule)
	MustInsertModule(ctx, t, testDB, stdlibModule)
	for _, test := range []struct {
		err                                 error
		name, fullPath, modulePath, version string
		want                                []*licenses.License
	}{
		{
			name:       "module root",
			fullPath:   sample.ModulePath,
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       []*licenses.License{testModule.Licenses[1]},
		},
		{
			name:       "package without license",
			fullPath:   sample.ModulePath + "/A",
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       []*licenses.License{testModule.Licenses[1]},
		},
		{
			name:       "package with additional license",
			fullPath:   sample.ModulePath + "/A/B",
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       testModule.Licenses,
		},
		{
			name:       "stdlib directory",
			fullPath:   "cmd",
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
		{
			name:       "stdlib package",
			fullPath:   "cmd/go",
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
		{
			name:       "stdlib module",
			fullPath:   stdlib.ModulePath,
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			u, err := testDB.GetUnit(ctx, newUnitMeta(test.fullPath, test.modulePath, test.version), internal.WithLicenses, internal.BuildContext{})
			if !errors.Is(err, test.err) {
				t.Fatal(err)
			}
			got := u.LicenseContents
			sort.Slice(got, func(i, j int) bool {
				return got[i].FilePath < got[j].FilePath
			})
			sort.Slice(test.want, func(i, j int) bool {
				return test.want[i].FilePath < test.want[j].FilePath
			})
			for i := range got {
				sort.Strings(got[i].Types)
			}
			for i := range test.want {
				sort.Strings(test.want[i].Types)
			}
			cmpopt := cmpopts.IgnoreFields(licenses.License{}, "Contents")
			if diff := cmp.Diff(test.want, got, cmpopt); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetModuleLicenses(t *testing.T) {
	t.Parallel()
	modulePath := "test.module"
	testModule := sample.Module(modulePath, "v1.2.3", "", "foo", "bar")
	testModule.Packages()[0].Licenses = []*licenses.Metadata{{Types: []string{"ISC"}, FilePath: "LICENSE"}}
	testModule.Packages()[1].Licenses = []*licenses.Metadata{{Types: []string{"MIT"}, FilePath: "foo/LICENSE"}}
	testModule.Packages()[2].Licenses = []*licenses.Metadata{{Types: []string{"GPL2"}, FilePath: "bar/LICENSE.txt"}}

	testDB, release := acquire(t)
	defer release()

	ctx := context.Background()

	testModule.Licenses = nil
	for _, p := range testModule.Packages() {
		testModule.Licenses = append(testModule.Licenses, &licenses.License{
			Metadata: p.Licenses[0],
			Contents: []byte(`Lorem Ipsum`),
		})
	}

	MustInsertModule(ctx, t, testDB, testModule)

	var moduleID int
	query := `
		SELECT m.id
		FROM modules m
		WHERE
		    m.module_path = $1
		    AND m.version = $2;`
	if err := testDB.db.QueryRow(ctx, query, modulePath, testModule.Version).Scan(&moduleID); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.getModuleLicenses(ctx, moduleID)
	if err != nil {
		t.Fatal(err)
	}
	// We only want the top-level license.
	wantLicenses := []*licenses.License{testModule.Licenses[0]}
	if diff := cmp.Diff(wantLicenses, got); diff != "" {
		t.Errorf("testDB.getModuleLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", modulePath, testModule.Version, diff)
	}
}

func TestGetLicensesBypass(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	bypassDB := NewBypassingLicenseCheck(testDB.db)

	// Insert with non-redistributable license contents.
	m := nonRedistributableModule()
	MustInsertModule(ctx, t, bypassDB, m)

	// check reads and the second license in the module and compares it with want.
	check := func(bypass bool, want *licenses.License) {
		t.Helper()
		db := testDB
		if bypass {
			db = bypassDB
		}
		u, err := db.GetUnit(ctx, newUnitMeta(sample.ModulePath, sample.ModulePath, m.Version), internal.WithLicenses, internal.BuildContext{})
		if err != nil {
			t.Fatal(err)
		}
		lics := u.LicenseContents
		if len(lics) != 2 {
			t.Fatal("did not read two licenses")
		}
		if diff := cmp.Diff(want, lics[1]); diff != "" {
			t.Errorf("mismatch (-want, +got):\n%s", diff)
		}
	}

	// Read with license bypass includes non-redistributable license contents.
	check(true, sample.NonRedistributableLicense)

	// Read without license bypass does not include non-redistributable license contents.
	nonRedist := *sample.NonRedistributableLicense
	nonRedist.Contents = nil
	check(false, &nonRedist)
}

func nonRedistributableModule() *internal.Module {
	m := sample.Module(sample.ModulePath, "v1.2.3", "")
	sample.AddLicense(m, sample.NonRedistributableLicense)
	m.IsRedistributable = false
	m.Packages()[0].IsRedistributable = false
	m.Units[0].IsRedistributable = false
	return m
}

// makeModuleNonRedistributable mutates the passed-in module by marking it
// non-redistributable along with each of its packages and units. It allows
// us to re-use existing test data without defining a non-redistributable
// counterpart to each.
func makeModuleNonRedistributable(m *internal.Module) {
	sample.AddLicense(m, sample.NonRedistributableLicense)
	m.IsRedistributable = false

	for _, p := range m.Packages() {
		p.IsRedistributable = false
	}

	for i := range m.Units {
		m.Units[i].IsRedistributable = false
	}
}
