// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

func TestGetNextModulesToFetchAndUpdateModuleVersionStatesForReprocessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	type testData struct {
		modulePath, version string
		numPackages, status int
	}
	var (
		latest    = "v1.2.0"
		notLatest = "v1.0.0"
		big       = 2000
		small     = 100
		versions  = []string{notLatest, latest}
		sizes     = []int{small, big}
		statuses  = []int{
			http.StatusOK,
			derrors.ToHTTPStatus(derrors.HasIncompletePackages),
			derrors.ToHTTPStatus(derrors.AlternativeModule),
			derrors.ToHTTPStatus(derrors.BadModule),
			http.StatusInternalServerError,
			http.StatusBadRequest,
		}
		indexVersions []*internal.IndexVersion
		now           = time.Now()
	)
	testPath := func(m *testData) string {
		return strings.ReplaceAll(fmt.Sprintf("%d/%d", m.numPackages, m.status), " ", "/")
	}
	generateMods := func(versions []string, sizes []int, statuses []int) []*testData {
		var mods []*testData
		for _, status := range statuses {
			for _, size := range sizes {
				for _, version := range versions {
					m := &testData{
						version:     version,
						numPackages: size,
						status:      status,
					}
					m.modulePath = testPath(m)
					mods = append(mods, m)
				}
			}
		}
		sort.Slice(mods, func(i, j int) bool {
			return mods[i].modulePath < mods[j].modulePath
		})
		return mods
	}
	mods := generateMods(versions, sizes, statuses)
	for _, data := range mods {
		indexVersions = append(indexVersions, &internal.IndexVersion{Path: data.modulePath, Version: data.version, Timestamp: now})
	}
	if err := testDB.InsertIndexVersions(ctx, indexVersions); err != nil {
		t.Fatal(err)
	}

	checkNextToRequeue := func(wantData []*testData) {
		t.Helper()
		got, err := testDB.GetNextModulesToFetch(ctx, len(mods)+1)
		if err != nil {
			t.Fatal(err)
		}

		var want []*internal.ModuleVersionState
		for _, data := range wantData {
			m := &internal.ModuleVersionState{
				ModulePath: data.modulePath,
				Version:    data.version,
				Status:     derrors.ToReprocessStatus(data.status),
			}
			if data.numPackages != 0 {
				m.NumPackages = &data.numPackages
			}
			want = append(want, m)
		}
		if len(want) != len(got) {
			t.Errorf("mismatch got = %d modules; want = %d", len(got), len(want))
		}
		ignore := cmpopts.IgnoreFields(
			internal.ModuleVersionState{},
			"AppVersion",
			"CreatedAt",
			"Error",
			"GoModPath",
			"IndexTimestamp",
			"LastProcessedAt",
			"NextProcessedAfter",
			"TryCount",
		)
		if diff := cmp.Diff(want, got, ignore); diff != "" {
			t.Fatalf("mismatch (-want, +got):\n%s", diff)
		}
	}
	updateStates := func(wantData []*testData) {
		for _, m := range wantData {
			if err := upsertModuleVersionState(ctx, testDB.db, m.modulePath, m.version, "2020-04-29t14", &m.numPackages, now, m.status,
				m.modulePath, derrors.FromHTTPStatus(m.status, "test string")); err != nil {
				t.Fatal(err)
			}
		}
	}

	// All of the modules should have status = 0, so they should all be
	// returned. At this point, we don't know the number of packages in each
	// module.
	want := generateMods([]string{latest}, []int{small, big}, statuses)
	sort.Slice(want, func(i, j int) bool {
		return want[i].modulePath < want[j].modulePath
	})
	want2 := generateMods([]string{notLatest}, []int{small, big}, statuses)
	sort.Slice(want2, func(i, j int) bool {
		return want2[i].modulePath < want2[j].modulePath
	})
	want = append(want, want2...)

	for _, w := range want {
		w.status = 0
		w.numPackages = 0
	}
	checkNextToRequeue(want)

	// Mark all modules for reprocessing.
	for _, m := range mods {
		if err := upsertModuleVersionState(ctx, testDB.db, m.modulePath, m.version, "2020-04-29t14", &m.numPackages, now, m.status, m.modulePath, derrors.FromHTTPStatus(m.status, "test string")); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.UpdateModuleVersionStatesForReprocessing(ctx, "2020-04-30t14"); err != nil {
		t.Fatal(err)
	}

	// The next modules to requeue should be only the latest version of
	// not-large modules with errors derorrs.ReprocessStatusOK and
	// derrors.ReprocessHasIncompletePackages.
	want = generateMods([]string{latest}, []int{small}, []int{200, 290})
	checkNextToRequeue(want)
	updateStates(want)

	// The next modules to requeue should be only the small latest version of
	// not-large modules with errors derorrs.ReprocessAlternative and
	// derrors.ReprocessBadModule.
	want = generateMods([]string{latest}, []int{small}, []int{490, 491})
	checkNextToRequeue(want)
	updateStates(want)

	// The next modules to requeue should be only the small non-latest
	// version of modules with errors derorrs.ReprocessStatusOK and
	// derrors.ReprocessHasIncompletePackages.
	want = generateMods([]string{notLatest}, []int{small}, []int{200, 290})
	checkNextToRequeue(want)
	updateStates(want)

	// The next modules to requeue should be only the small non-latest
	// version of modules with errors derorrs.ReprocessAlternative and //
	// derrors.ReprocessBadModule.
	want = generateMods([]string{notLatest}, []int{small}, []int{490, 491})
	checkNextToRequeue(want)
	updateStates(want)

	// Pick up all large modules. Modules with 520 > status >= 500
	// have already been processsed.
	tmp := generateMods(versions, []int{big}, []int{200, 290})
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].version > tmp[j].version
	})
	want = tmp
	tmp = generateMods(versions, []int{big}, []int{490, 491})
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].version > tmp[j].version
	})
	want = append(want, tmp...)
	checkNextToRequeue(want)
	updateStates(want)

	// At this point, everything should have been queued.
	// Modules with status=500 and status=400 are not being picked up because
	// their next_processed_at time > NOW.
	checkNextToRequeue(nil)
}

func TestGetNextModulesToFetchOnlyPicksUpStatus0AndStatusGreaterThan500(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	statuses := []int{
		http.StatusOK,
		derrors.ToHTTPStatus(derrors.HasIncompletePackages),
		derrors.ToHTTPStatus(derrors.AlternativeModule),
		derrors.ToHTTPStatus(derrors.BadModule),
		http.StatusBadRequest,
		http.StatusInternalServerError,
		0,
	}
	for _, status := range statuses {
		if _, err := testDB.db.Exec(ctx, `
			INSERT INTO module_version_states AS mvs (
				module_path,
				version,
				sort_version,
				app_version,
				index_timestamp,
				status,
				go_mod_path)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			strconv.Itoa(status),
			"v1.0.0",
			version.ForSorting("v1.0.0"),
			"app-version",
			time.Now(),
			status,
			strconv.Itoa(status),
		); err != nil {
			t.Fatal(err)
		}
	}

	got, err := testDB.GetNextModulesToFetch(ctx, len(statuses))
	if err != nil {
		t.Fatal(err)
	}
	var (
		want         []*internal.ModuleVersionState
		wantStatuses = []int{http.StatusInternalServerError, 0}
	)
	for _, status := range wantStatuses {
		m := &internal.ModuleVersionState{
			ModulePath: strconv.Itoa(status),
			Version:    "v1.0.0",
			Status:     status,
		}
		want = append(want, m)
	}
	compareModules(t, got, want)
}

func TestGetNextModulesToFetchLargeModulesLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	var (
		modulePaths []string
		status      = http.StatusInternalServerError
		v           = "v1.0.0"
		num500Mods  = 10
	)
	for i := 0; i < num500Mods; i++ {
		mp := strconv.Itoa(status) + strconv.Itoa(i)
		modulePaths = append(modulePaths, mp)
		if _, err := testDB.db.Exec(ctx, `
			INSERT INTO module_version_states AS mvs (
				module_path,
				version,
				sort_version,
				app_version,
				index_timestamp,
				status,
				go_mod_path)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			mp,
			v,
			version.ForSorting(v),
			"app-version",
			time.Now(),
			status,
			strconv.Itoa(status),
		); err != nil {
			t.Fatal(err)
		}
	}

	largeModulesLimit = 5
	got, err := testDB.GetNextModulesToFetch(ctx, num500Mods)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(modulePaths)
	var want []*internal.ModuleVersionState
	for _, mp := range modulePaths[:largeModulesLimit] {
		want = append(want, &internal.ModuleVersionState{
			ModulePath: mp,
			Version:    v,
			Status:     status,
		})
	}
	compareModules(t, got, want)
}

func compareModules(t *testing.T, got, want []*internal.ModuleVersionState) {
	t.Helper()
	ignore := cmpopts.IgnoreFields(
		internal.ModuleVersionState{},
		"AppVersion",
		"CreatedAt",
		"Error",
		"GoModPath",
		"IndexTimestamp",
		"LastProcessedAt",
		"NextProcessedAfter",
		"TryCount",
	)
	sort.Slice(got, func(i, j int) bool {
		return got[i].ModulePath < got[j].ModulePath
	})
	sort.Slice(want, func(i, j int) bool {
		return want[i].ModulePath < want[j].ModulePath
	})
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Fatalf("mismatch (-want, +got):\n%s", diff)
	}
}
