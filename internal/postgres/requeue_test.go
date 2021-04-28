// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"bytes"
	"context"
	"crypto/md5"
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	type testData struct {
		modulePath, version string
		numPackages, status int
	}
	var (
		latest       = "v1.5.2"
		prerelease   = "v1.5.2-prerelease"
		nonLatest    = "v1.5.1"
		incompatible = "v2.0.0+incompatible"
		big          = 2000
		small        = 100
		versions     = []string{incompatible, latest, nonLatest, prerelease}
		sizes        = []int{small, big}
		statuses     = []int{
			http.StatusOK,
			derrors.ToStatus(derrors.HasIncompletePackages),
			derrors.ToStatus(derrors.DBModuleInsertInvalid),
			http.StatusInternalServerError,
			http.StatusBadRequest,
		}
		indexVersions []*internal.IndexVersion
		now           = time.Now()
	)
	generateMods := func(vers []string, sizes, statuses []int) []*testData {
		var mods []*testData
		for _, status := range statuses {
			for _, size := range sizes {
				for _, version := range vers {
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

	sortNum := func(m *internal.ModuleVersionState) int {
		if m.Status == 0 {
			return 0
		}
		var s int
		switch m.Status {
		case 503, 520, 521:
			s = 1
		case 540, 541, 542:
			s = 2
		default:
			s = 5
		}
		if m.Version != latest && s < 5 {
			s += 2
		}
		return s
	}

	mvLess := func(m1, m2 *internal.ModuleVersionState) bool {
		n1 := sortNum(m1)
		n2 := sortNum(m2)
		if n1 != n2 {
			return n1 < n2
		}
		return bytes.Compare(md5hash(m1), md5hash(m2)) < 0
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
		sort.Slice(want, func(i, j int) bool { return mvLess(want[i], want[j]) })
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

	// All of the modules should have status = 0, so they should all be
	// returned. At this point, we don't know the number of packages in each
	// module.
	want := generateMods(versions, sizes, statuses)
	for _, w := range want {
		w.status = 0
		w.numPackages = 0
	}
	checkNextToRequeue(want, len(mods))

	for _, m := range mods {
		mvs := &ModuleVersionStateForUpsert{
			ModulePath: m.modulePath,
			Version:    m.version,
			AppVersion: "2020-04-29t14",
			Timestamp:  now,
			Status:     m.status,
			GoModPath:  m.modulePath,
			FetchErr:   derrors.FromStatus(m.status, "test string"),
		}
		if err := upsertModuleVersionState(ctx, testDB.db, &m.numPackages, mvs); err != nil {
			t.Fatal(err)
		}
	}

	// Mark the latest version of all modules for reprocessing.
	if err := testDB.UpdateModuleVersionStatesForReprocessingLatestOnly(ctx, "2020-04-30t14"); err != nil {
		t.Fatal(err)
	}
	want = generateMods([]string{latest}, sizes, []int{200, 290})
	checkNextToRequeue(want, len(mods))

	// Mark the release and non-incompatible version of all modules for reprocessing.
	if err := testDB.UpdateModuleVersionStatesForReprocessingReleaseVersionsOnly(ctx, "2020-04-30t14"); err != nil {
		t.Fatal(err)
	}
	want = generateMods([]string{latest, nonLatest}, sizes, []int{200, 290})
	checkNextToRequeue(want, len(mods))

	// Mark all modules for reprocessing.
	if err := testDB.UpdateModuleVersionStatesForReprocessing(ctx, "2020-04-30t14"); err != nil {
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

	want = generateMods(versions, sizes, []int{200, 290, 480, 500})
	checkNextToRequeue(want, len(mods))
}

func md5hash(mv *internal.ModuleVersionState) []byte {
	s := md5.Sum([]byte(mv.ModulePath + mv.Version))
	return s[:]
}
func TestGetNextModulesToFetchOnlyPicksUpStatus0AndStatusGreaterThan500(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

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
