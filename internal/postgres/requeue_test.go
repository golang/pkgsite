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
		latest    = "v1.5.2"
		notLatest = "v2.0.0+incompatible"
		big       = 2000
		small     = 100
		versions  = []string{notLatest, latest}
		sizes     = []int{small, big}
		statuses  = []int{
			http.StatusOK,
			derrors.ToStatus(derrors.HasIncompletePackages),
			derrors.ToStatus(derrors.AlternativeModule),
			derrors.ToStatus(derrors.BadModule),
			derrors.ToStatus(derrors.DBModuleInsertInvalid),
			http.StatusInternalServerError,
			http.StatusBadRequest,
		}
		indexVersions []*internal.IndexVersion
		now           = time.Now()
	)
	generateMods := func(versions []string, sizes, statuses []int) []*testData {
		var mods []*testData
		for _, status := range statuses {
			for _, size := range sizes {
				for _, version := range versions {
					mods = append(mods, &testData{
						modulePath:  fmt.Sprintf("%d/%d", size, status),
						version:     version,
						numPackages: size,
						status:      status,
					})
				}
			}
		}
		sort.Slice(mods, func(i, j int) bool {
			if mods[i].modulePath != mods[j].modulePath {
				return mods[i].modulePath < mods[j].modulePath
			}
			return mods[i].version < mods[j].version
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

	checkNextToRequeue := func(wantData []*testData, limit int) {
		t.Helper()
		got, err := testDB.GetNextModulesToFetch(ctx, limit)
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
				m.modulePath, derrors.FromStatus(m.status, "test string")); err != nil {
				t.Fatal(err)
			}
		}
	}

	// All of the modules should have status = 0, so they should all be
	// returned. At this point, we don't know the number of packages in each
	// module.
	want := generateMods(versions, sizes, statuses)
	for _, w := range want {
		w.status = 0
		w.numPackages = 0
	}
	checkNextToRequeue(want, len(mods))
	// Mark all modules for reprocessing.
	for _, m := range mods {
		if err := upsertModuleVersionState(ctx, testDB.db, m.modulePath, m.version, "2020-04-29t14", &m.numPackages, now, m.status, m.modulePath, derrors.FromStatus(m.status, "test string")); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.UpdateModulesForReprocessing(ctx, "2020-04-30t14"); err != nil {
		t.Fatal(err)
	}
	// Set the next-processed time for everything to now. UpdateModuleVersionStatesForReprocessing does
	// that for some modules, but we need to do it for all of them so that they all are candidates
	// for GetNextModulesToFetch.
	if _, err := testDB.db.Exec(ctx, `
		UPDATE module_version_states
		SET next_processed_after = CURRENT_TIMESTAMP
	`); err != nil {
		t.Fatal(err)
	}

	// The first modules to requeue should be the latest version of not-large modules with errors
	// ReprocessStatusOK ReprocessHasIncompletePackages, ReprocessAlternative, and ReprocessBadModule.
	statuses = []int{200, 290, 480, 490, 491}
	want = generateMods([]string{latest}, []int{small}, statuses)

	// The next modules to requeue should be the small non-latest versions.
	want = append(want, generateMods([]string{notLatest}, []int{small}, statuses)...)
	// Next, latest large modules.
	want = append(want, generateMods([]string{latest}, []int{big}, statuses)...)
	// Lastly, not-latest large modules.
	want = append(want, generateMods([]string{notLatest}, []int{big}, statuses)...)

	want = append(want, generateMods([]string{latest, notLatest}, []int{small, big}, []int{500})...)
	checkNextToRequeue(want, len(mods))

	// Take modules in groups by passing a limit.
	const limit = 6
	for i := 0; i < len(mods); i += limit {
		end := i + limit
		if end > len(want) {
			end = len(want)
		}
		w := want[i:end]
		checkNextToRequeue(w, limit)
		updateStates(w)
	}

	// At this point, everything should have been queued except modules with
	// status=400 and >= 500, and the latter have a next_processed_after time
	// that is in the future.
	checkNextToRequeue(nil, 5)
}

func TestGetNextModulesToFetchOnlyPicksUpStatus0AndStatusGreaterThan500(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	statuses := []int{
		http.StatusOK,
		derrors.ToStatus(derrors.HasIncompletePackages),
		derrors.ToStatus(derrors.AlternativeModule),
		derrors.ToStatus(derrors.BadModule),
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
				go_mod_path,
				incompatible)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			strconv.Itoa(status),
			"v1.0.0",
			version.ForSorting("v1.0.0"),
			"app-version",
			time.Now(),
			status,
			strconv.Itoa(status),
			false,
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
		numPackages = largeModulePackageThreshold + 1
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
				num_packages,
				incompatible)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			mp,
			v,
			version.ForSorting(v),
			"app-version",
			time.Now(),
			status,
			numPackages,
			isIncompatible(v),
		); err != nil {
			t.Fatal(err)
		}
	}

	largeModulesLimit = num500Mods / 2
	got, err := testDB.GetNextModulesToFetch(ctx, num500Mods)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(modulePaths)
	var want []*internal.ModuleVersionState
	for _, mp := range modulePaths[:largeModulesLimit] {
		want = append(want, &internal.ModuleVersionState{
			ModulePath:  mp,
			Version:     v,
			Status:      status,
			NumPackages: &numPackages,
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
