// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetVersions(t *testing.T) {
	t.Parallel()
	var (
		taggedAndPseudoModule = "path.to/foo"
		taggedModuleV2        = "path.to/foo/v2"
		taggedModuleV3        = "path.to/foo/v3"
		pseudoModule          = "golang.org/x/tools"
		otherModule           = "path.to/other"
		incompatibleModule    = "path.to/incompatible"
		rootModule            = "golang.org/foo/bar"
		nestedModule          = "golang.org/foo/bar/api"
		testModules           = []*internal.Module{
			sample.Module(stdlib.ModulePath, "v1.15.0-beta.1", "cmd/go"),
			sample.Module(stdlib.ModulePath, "v1.14.6", "cmd/go"),
			sample.Module(taggedModuleV3, "v3.2.0-beta", "bar"),
			sample.Module(taggedModuleV3, "v3.2.0-alpha.2", "bar"),
			sample.Module(taggedModuleV3, "v3.2.0-alpha.1", "bar"),
			sample.Module(taggedModuleV3, "v3.1.0", "bar"),
			sample.Module(taggedModuleV3, "v3.0.0", "bar"),
			sample.Module(taggedModuleV2, "v2.0.1", "bar"),
			sample.Module(taggedAndPseudoModule, "v1.5.3-pre1", "bar"),
			sample.Module(taggedAndPseudoModule, "v1.5.2", "bar"),
			sample.Module(taggedAndPseudoModule, "v0.0.0-20200101120000-000000000000", "bar"),
			sample.Module(otherModule, "v3.0.0", "thing"),
			sample.Module(incompatibleModule, "v2.0.0+incompatible", "module"),
			sample.Module(incompatibleModule, "v0.0.0", "module"),
			sample.Module(rootModule, "v1.0.3", "api"),
			sample.Module(rootModule, "v0.11.6", "api"),
			sample.Module(nestedModule, "v1.0.4", "api"),
			sample.Module(nestedModule, "v1.0.3", "api"),
		}
	)

	// Add 12 pseudo versions to the test modules. Below we only
	// expect to return the 10 most recent.
	for i := 1; i <= 12; i++ {
		testModules = append(testModules, sample.Module(pseudoModule, fmt.Sprintf("v0.0.0-202001011200%02d-000000000000", i), "blog"))
	}

	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, m := range testModules {
		MustInsertModule(ctx, t, testDB, m)
	}
	// Add latest version info for rootModule.
	addLatest(ctx, t, testDB, rootModule, "v1.1.0", `
		module golang.org/foo/bar // Deprecated: use other
		retract v1.0.3 // security flaw
    `)

	stdModuleVersions := []*internal.ModuleInfo{
		{
			ModulePath: stdlib.ModulePath,
			Version:    "v1.15.0-beta.1",
		},
		{
			ModulePath: stdlib.ModulePath,
			Version:    "v1.14.6",
		},
	}

	fooModuleVersions := []*internal.ModuleInfo{
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-beta",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-alpha.2",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-alpha.1",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.1.0",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.0.0",
		},
		{
			ModulePath: taggedModuleV2,
			Version:    "v2.0.1",
		},
		{
			ModulePath: taggedAndPseudoModule,
			Version:    "v1.5.3-pre1",
		},
		{
			ModulePath: taggedAndPseudoModule,
			Version:    "v1.5.2",
		},
	}

	testCases := []struct {
		name, path string
		want       []*internal.ModuleInfo
	}{
		{
			name: "std_module",
			path: stdlib.ModulePath,
			want: stdModuleVersions,
		},
		{
			name: "stdlib_path",
			path: "cmd",
			want: stdModuleVersions,
		},
		{
			name: "stdlib_package",
			path: "cmd/go",
			want: stdModuleVersions,
		},
		{
			name: "want_tagged_versions_only",
			path: "path.to/foo/bar",
			want: fooModuleVersions,
		},
		{
			name: "want_all_tagged_versions_at_v2_path",
			path: "path.to/foo/v2/bar",
			want: fooModuleVersions,
		},
		{
			name: "want_pseudo_versions_only",
			path: "golang.org/x/tools/blog",
			want: func() []*internal.ModuleInfo {
				versions := []*internal.ModuleInfo{}
				// Expect the 10 most recent in DESC order
				for i := 12; i > 2; i-- {
					versions = append(versions, &internal.ModuleInfo{
						ModulePath: pseudoModule,
						Version:    fmt.Sprintf("v0.0.0-202001011200%02d-000000000000", i),
					})
				}
				return versions
			}(),
		},
		{
			name: "want_tagged_versions_includes_incompatible",
			path: "path.to/incompatible/module",
			want: []*internal.ModuleInfo{
				{
					ModulePath: incompatibleModule,
					Version:    "v0.0.0",
				},
				{
					ModulePath: incompatibleModule,
					Version:    "v2.0.0+incompatible",
				},
			},
		},
		{
			name: "nested_module_path",
			path: "golang.org/foo/bar/api",
			want: []*internal.ModuleInfo{
				{
					ModulePath: nestedModule,
					Version:    "v1.0.4",
				},
				{
					ModulePath: nestedModule,
					Version:    "v1.0.3",
				},
				{
					ModulePath:          rootModule,
					Version:             "v1.0.3",
					Deprecated:          true,
					DeprecationComment:  "use other",
					Retracted:           true,
					RetractionRationale: "security flaw",
				},
				{
					ModulePath:         rootModule,
					Version:            "v0.11.6",
					Deprecated:         true,
					DeprecationComment: "use other",
				},
			},
		},
		{
			name: "root module path",
			path: "golang.org/foo/bar",
			want: []*internal.ModuleInfo{
				{
					ModulePath:          rootModule,
					Version:             "v1.0.3",
					Deprecated:          true,
					DeprecationComment:  "use other",
					Retracted:           true,
					RetractionRationale: "security flaw",
				},
				{
					ModulePath:         rootModule,
					Version:            "v0.11.6",
					Deprecated:         true,
					DeprecationComment: "use other",
				},
			},
		},
		{
			name: "want_zero_results_in_non_empty_db",
			path: "not.a/real/path",
		},
	}

	ctx = experiment.NewContext(ctx, internal.ExperimentRetractions)
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			for _, w := range test.want {
				mod := sample.ModuleInfo(w.ModulePath, w.Version)
				w.CommitTime = mod.CommitTime
				w.IsRedistributable = mod.IsRedistributable
				w.HasGoMod = mod.HasGoMod
				w.SourceInfo = mod.SourceInfo
			}

			got, err := testDB.GetVersionsForPath(ctx, test.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetVersionsForPath(%q) mismatch (-want +got):\n%s", test.path, diff)
			}
		})
	}
}

func TestGetLatestInfo(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, m := range []*internal.Module{
		sample.Module("a.com/M", "v99.0.0+incompatible", "all", "most"),
		sample.Module("a.com/M", "v1.1.1", "all", "most", "some", "one", "D/other"),
		sample.Module("a.com/M", "v1.2.0", "all", "most"),
		sample.Module("a.com/M/v2", "v2.0.5", "all", "most"),
		sample.Module("a.com/M/v3", "v3.0.1", "all", "some"),
		sample.Module("a.com/M/D", "v1.3.0", "other"),
		sample.Module("b.com/M/v9", "v9.0.0", ""),
		sample.Module("b.com/M/v10", "v10.0.0", ""),
		sample.Module("gopkg.in/M.v1", "v1.0.0", ""),
		sample.Module("gopkg.in/M.v2", "v2.0.0-pre", ""),
		sample.Module("gopkg.in/M.v3", "v3.0.0-20200602140019-6ec2bf8d378b", ""),
		sample.Module("c.com/M", "v0.0.0-20200602140019-6ec2bf8d378b", ""),
	} {
		MustInsertModuleLatest(ctx, t, testDB, m)
	}

	for _, test := range []struct {
		unit, module string
		want         internal.LatestInfo
	}{
		{
			// A unit that is the module.
			"a.com/M", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit that exists in all versions of the module.
			"a.com/M/all", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3/all",
			},
		},
		{
			// A unit that exists in most versions, but not the latest major.
			"a.com/M/most", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit that does not exist at the latest minor version, but does at the latest major.
			"a.com/M/some", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.1.1",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3/some",
			},
		},
		{
			// A unit that does not exist at the latest minor or major versions.
			"a.com/M/one", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.1.1",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit whose latest minor version is in a different module.
			"a.com/M/D/other", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.3.0",
				MinorModulePath:   "a.com/M/D",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A module with v9 and v10 versions.
			"b.com/M/v9", "b.com/M/v9",
			internal.LatestInfo{
				MinorVersion:      "v9.0.0",
				MinorModulePath:   "b.com/M/v9",
				UnitExistsAtMinor: true,
				MajorModulePath:   "b.com/M/v10",
				MajorUnitPath:     "b.com/M/v10",
			},
		},
		{
			// gopkg.in module, highest version is not tagged
			"gopkg.in/M.v1", "gopkg.in/M.v1",
			internal.LatestInfo{
				MinorVersion:      "v1.0.0",
				MinorModulePath:   "gopkg.in/M.v1",
				UnitExistsAtMinor: true,
				MajorModulePath:   "gopkg.in/M.v2",
				MajorUnitPath:     "gopkg.in/M.v2",
			},
		},
		{
			// no tagged versions
			"c.com/M", "c.com/M",
			internal.LatestInfo{
				MinorVersion:      "v0.0.0-20200602140019-6ec2bf8d378b",
				MinorModulePath:   "c.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "",
				MajorUnitPath:     "",
			},
		},
	} {
		t.Run(test.unit, func(t *testing.T) {
			got, err := testDB.GetLatestInfo(ctx, test.unit, test.module, nil)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestRawIsMoreRecent(t *testing.T) {
	for _, test := range []struct {
		new, cur string
		want     bool
	}{
		{"v1.2.0", "v1.0.0", true},
		{"v1.2.0", "v1.2.0", false},
		{"v1.2.0", "v1.3.0", false},
		{"v1.0.0", "v1.9.9-pre", true},          // release beats prerelease
		{"v1.0.0", "v2.3.4+incompatible", true}, // compatible beats incompatible
	} {
		got := rawIsMoreRecent(test.new, test.cur)
		if got != test.want {
			t.Errorf("rawIsMoreRecent(%q, %q) = %t, want %t", test.new, test.cur, got, test.want)
		}
	}
}

func TestGetLatestGoodVersion(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const (
		modulePath = "example.com/m"
		modFile    = `
			module example.com/m
			retract v1.3.0
		`
	)

	lmv, err := internal.NewLatestModuleVersions(modulePath, "", "", "", []byte(modFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"v2.0.0+incompatible", "v1.4.0-pre", "v1.3.0", "v1.2.0", "v1.1.0"} {
		MustInsertModuleLatest(ctx, t, testDB, sample.Module(modulePath, v, "pkg"))
	}

	for _, test := range []struct {
		name   string
		cooked string // cooked latest version; empty means nil lmv
		want   string
	}{
		{
			name:   "compatible",
			cooked: "v1.3.0",
			want:   "v1.2.0", // v1.3.0 is retracted
		},
		{
			name:   "incompatible",
			cooked: "v2.0.0+incompatible",
			want:   "v2.0.0+incompatible",
		},
		{
			name:   "bad", // cooked version not in modules table
			cooked: "v1.4.0",
			want:   "v1.2.0",
		},
		{
			name:   "nil",
			cooked: "", // no latest-version info
			want:   "v2.0.0+incompatible",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var lm *internal.LatestModuleVersions
			if test.cooked != "" {
				lmv.CookedVersion = test.cooked
				lm = lmv
			}
			got, err := getLatestGoodVersion(ctx, testDB.db, modulePath, lm)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestLatestModuleVersions(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const (
		modulePath = "example.com/m"
		modFile    = `
			module example.com/m
			retract v1.3.0
		`
		incompatible = "v2.0.0+incompatible"
	)
	modBytes := []byte(modFile)

	type versions struct {
		raw, cooked string
	}
	// These tests form a sequence. Each test's want versions are in the DB for the next test.
	for _, test := range []struct {
		name     string
		in, want versions
	}{
		{
			name: "initial",
			in:   versions{"v1.3.0", "v1.2.0"},
			want: versions{"v1.3.0", "v1.2.0"},
		},
		{
			name: "older", // older incoming info doesn't cause an update
			in:   versions{"v1.2.0", "v1.2.0"},
			want: versions{"v1.3.0", "v1.2.0"},
		},
		{
			name: "newer bad", // a newer version, not in modules table
			in:   versions{"v1.4.0", "v1.4.0"},
			want: versions{"v1.4.0", "v1.4.0"},
		},
		{
			name: "incompatible",
			in:   versions{incompatible, incompatible},
			want: versions{incompatible, incompatible},
		},
		{
			name: "compatible", // "downgrade" from incompatible to compatible will update
			in:   versions{"v1.3.0", "v1.2.0"},
			want: versions{"v1.3.0", "v1.2.0"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			vNew, err := internal.NewLatestModuleVersions(modulePath, test.in.raw, test.in.cooked, "", modBytes)
			if err != nil {
				t.Fatal(err)
			}
			vGot, err := testDB.UpdateLatestModuleVersions(ctx, vNew)
			if err != nil {
				t.Fatal(err)
			}
			if vGot == nil {
				t.Fatal("got nothing")
			}
			got := versions{vGot.RawVersion, vGot.CookedVersion}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestLatestModuleVersionsBadStatus(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const modulePath = "example.com/m"

	getStatus := func() int {
		var s int
		err := testDB.db.QueryRow(ctx, `
				SELECT status
				FROM latest_module_versions l
				INNER JOIN paths p ON (p.id=l.module_path_id)
				WHERE p.path = $1`,
			modulePath).Scan(&s)
		if err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		return s
	}

	// Insert a failure status.
	newStatus := 410
	if err := testDB.UpdateLatestModuleVersionsStatus(ctx, modulePath, newStatus); err != nil {
		t.Fatal(err)
	}
	if got := getStatus(); got != newStatus {
		t.Errorf("got %d, want %d", got, newStatus)
	}

	// GetLatestModuleVersions should return nil.
	got, err := testDB.GetLatestModuleVersions(ctx, modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}

	// A new failure status should overwrite.
	newStatus = 404
	if err := testDB.UpdateLatestModuleVersionsStatus(ctx, modulePath, newStatus); err != nil {
		t.Fatal(err)
	}
	if got := getStatus(); got != newStatus {
		t.Errorf("got %d, want %d", got, newStatus)
	}

	// Good information overwrites.
	lmv, err := internal.NewLatestModuleVersions(modulePath, "v1.2.3", "v1.2.3", "", []byte(`module example.com/m`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.UpdateLatestModuleVersions(ctx, lmv); err != nil {
		t.Fatal(err)
	}
	if got := getStatus(); got != 200 {
		t.Errorf("got %d, want %d", got, 200)
	}

	// Once we have good information, a bad status won't remove it.
	if err := testDB.UpdateLatestModuleVersionsStatus(ctx, modulePath, 500); err != nil {
		t.Fatal(err)
	}
	got, err = testDB.GetLatestModuleVersions(ctx, modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("got nil, want non-nil")
	}
}

func TestLatestModuleVersionsGood(t *testing.T) {
	// Verify that the good latest version is updated properly.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	const modulePath = "example.com/m"

	check := func(want string) {
		t.Helper()
		got, err := testDB.GetLatestModuleVersions(ctx, modulePath)
		if err != nil {
			t.Fatal(err)
		}
		if got.GoodVersion != want {
			t.Fatalf("got %q, want %q", got.GoodVersion, want)
		}
	}

	// Add two good versions.
	v1 := "v1.1.0"
	lmv := addLatest(ctx, t, testDB, modulePath, v1, "")
	MustInsertModuleLMV(ctx, t, testDB, sample.Module(modulePath, v1, "pkg"), lmv)
	check(v1)

	// Good version should be updated.
	v2 := "v1.2.0"
	lmv = addLatest(ctx, t, testDB, modulePath, v2, "")
	MustInsertModuleLMV(ctx, t, testDB, sample.Module(modulePath, v2, "pkg"), lmv)
	check(v2)

	// New latest-version info retracts v2; good version should switch to v1.
	addLatest(ctx, t, testDB, modulePath, "v1.3.0", fmt.Sprintf("module %s\nretract %s", modulePath, v2))
	check(v1)
}
