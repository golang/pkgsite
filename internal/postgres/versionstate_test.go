// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestModuleVersionState(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// verify that latest index timestamp works
	initialTime, err := testDB.LatestIndexTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := (time.Time{}); initialTime != want {
		t.Errorf("testDB.LatestIndexTimestamp(ctx) = %v, want %v", initialTime, want)
	}

	now := sample.NowTruncated()
	latest := now.Add(10 * time.Second)
	// insert a FooVersion with no Timestamp, to ensure that it is later updated
	// on conflict.
	initialFooVersion := &internal.IndexVersion{
		Path:    "foo.com/bar",
		Version: "v1.0.0",
	}
	if err := testDB.InsertIndexVersions(ctx, []*internal.IndexVersion{initialFooVersion}); err != nil {
		t.Fatal(err)
	}
	fooVersion := &internal.IndexVersion{
		Path:      "foo.com/bar",
		Version:   "v1.0.0",
		Timestamp: now,
	}
	bazVersion := &internal.IndexVersion{
		Path:      "baz.com/quux",
		Version:   "v2.0.1",
		Timestamp: latest,
	}
	versions := []*internal.IndexVersion{fooVersion, bazVersion}
	if err := testDB.InsertIndexVersions(ctx, versions); err != nil {
		t.Fatal(err)
	}

	gotVersions, err := testDB.GetNextModulesToFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	wantVersions := []*internal.ModuleVersionState{
		{ModulePath: "foo.com/bar", Version: "v1.0.0", IndexTimestamp: fooVersion.Timestamp},
		{ModulePath: "baz.com/quux", Version: "v2.0.1", IndexTimestamp: bazVersion.Timestamp},
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleVersionState{}, "CreatedAt", "LastProcessedAt", "NextProcessedAfter")
	if diff := cmp.Diff(wantVersions, gotVersions, ignore); diff != "" {
		t.Fatalf("testDB.GetVersionsToFetch(ctx, 10) mismatch (-want +got):\n%s", diff)
	}

	var (
		statusCode      = 500
		fetchErr        = errors.New("bad request")
		goModPath       = "goModPath"
		pkgVersionState = &internal.PackageVersionState{
			ModulePath:  "foo.com/bar",
			PackagePath: "foo.com/bar/foo",
			Version:     "v1.0.0",
			Status:      500,
		}
	)
	mvs := &ModuleVersionStateForUpsert{
		ModulePath:           fooVersion.Path,
		Version:              fooVersion.Version,
		Timestamp:            fooVersion.Timestamp,
		Status:               statusCode,
		GoModPath:            goModPath,
		FetchErr:             fetchErr,
		PackageVersionStates: []*internal.PackageVersionState{pkgVersionState},
	}
	if err := testDB.UpsertModuleVersionState(ctx, mvs); err != nil {
		t.Fatal(err)
	}
	errString := fetchErr.Error()
	numPackages := 1
	wantFooState := &internal.ModuleVersionState{
		ModulePath:     "foo.com/bar",
		Version:        "v1.0.0",
		IndexTimestamp: now,
		TryCount:       1,
		GoModPath:      goModPath,
		Error:          errString,
		Status:         statusCode,
		NumPackages:    &numPackages,
	}
	gotFooState, err := testDB.GetModuleVersionState(ctx, wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantFooState, gotFooState, ignore); diff != "" {
		t.Errorf("testDB.GetModuleVersionState(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	gotPVS, err := testDB.GetPackageVersionState(ctx, pkgVersionState.PackagePath, pkgVersionState.ModulePath, pkgVersionState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(pkgVersionState, gotPVS); diff != "" {
		t.Errorf("testDB.GetPackageVersionStates(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	gotPkgVersionStates, err := testDB.GetPackageVersionStatesForModule(ctx,
		wantFooState.ModulePath, wantFooState.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]*internal.PackageVersionState{pkgVersionState}, gotPkgVersionStates); diff != "" {
		t.Errorf("testDB.GetPackageVersionStates(ctx, %q, %q) mismatch (-want +got)\n%s", wantFooState.ModulePath, wantFooState.Version, diff)
	}

	stats, err := testDB.GetVersionStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantStats := &VersionStats{
		LatestTimestamp: latest,
		VersionCounts: map[int]int{
			0:   1,
			500: 1,
		},
	}
	if diff := cmp.Diff(wantStats, stats); diff != "" {
		t.Errorf("testDB.GetVersionStats(ctx) mismatch (-want +got):\n%s", diff)
	}

	if _, err := testDB.GetRecentFailedVersions(ctx, 10); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertModuleVersionStates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	m := sample.DefaultModule()
	appVersion := time.Now().String()

	for _, test := range []struct {
		name                  string
		shouldInsertModule    bool
		insertModuleBeforeMVS bool
		status                int
		wantUpsertMVSError    bool
		wantMVSStatus         int
		wantModulesStatus     int
	}{
		{
			name:                  "upsert mvs without inserting module, status 200",
			shouldInsertModule:    false,
			insertModuleBeforeMVS: false,
			status:                http.StatusOK,
			wantUpsertMVSError:    false,
			wantMVSStatus:         http.StatusOK,
			wantModulesStatus:     0,
		},
		{
			name:                  "upsert mvs without inserting module, status 400",
			shouldInsertModule:    false,
			insertModuleBeforeMVS: false,
			status:                http.StatusBadRequest,
			wantUpsertMVSError:    false,
			wantMVSStatus:         http.StatusBadRequest,
			wantModulesStatus:     0,
		},
		{
			name:                  "upsert mvs after inserting module, status 200",
			shouldInsertModule:    true,
			insertModuleBeforeMVS: true,
			status:                http.StatusOK,
			wantUpsertMVSError:    false,
			wantMVSStatus:         http.StatusOK,
			wantModulesStatus:     http.StatusOK,
		},
		{
			name:                  "upsert mvs after inserting module, status 400",
			shouldInsertModule:    true,
			insertModuleBeforeMVS: true,
			status:                http.StatusBadRequest,
			wantUpsertMVSError:    false,
			wantMVSStatus:         http.StatusBadRequest,
			wantModulesStatus:     http.StatusBadRequest,
		},
		{
			name:                  "upsert mvs before inserting module, status 200",
			shouldInsertModule:    true,
			insertModuleBeforeMVS: false,
			status:                http.StatusOK,
			wantUpsertMVSError:    false,
			wantMVSStatus:         http.StatusOK,
			wantModulesStatus:     0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			if test.insertModuleBeforeMVS && test.shouldInsertModule {
				MustInsertModule(ctx, t, testDB, m)
			}

			mvsu := &ModuleVersionStateForUpsert{
				ModulePath: m.ModulePath,
				Version:    m.Version,
				AppVersion: appVersion,
				Timestamp:  time.Now(),
				Status:     test.status,
				HasGoMod:   true,
			}
			err := testDB.UpsertModuleVersionState(ctx, mvsu)
			if test.wantUpsertMVSError != (err != nil) {
				t.Fatalf("db.UpsertModuleVersionState(): %v, want error: %t", err, test.wantUpsertMVSError)
			}
			mvs, err := testDB.GetModuleVersionState(ctx, m.ModulePath, m.Version)
			if err != nil {
				t.Fatalf("db.GetModuleVersionState(): %v", err)
			}
			if mvs.Status != test.wantMVSStatus {
				t.Errorf("module_version_states.status = %d, want %d", mvs.Status, test.wantMVSStatus)
			}
			if !mvs.HasGoMod {
				t.Errorf("HasGoMod: got %t, want true", mvs.HasGoMod)
			}

			if !test.insertModuleBeforeMVS && test.shouldInsertModule {
				MustInsertModule(ctx, t, testDB, m)
			}

			if !test.shouldInsertModule {
				return
			}

			var gotStatus sql.NullInt64
			err = testDB.db.QueryRow(ctx, `
                    SELECT status
                    FROM modules
                    WHERE module_path = $1 AND version = $2;`,
				m.ModulePath, m.Version).Scan(&gotStatus)
			if err != nil {
				t.Fatalf("db.QueryRow(): %v", err)
			}
			if test.insertModuleBeforeMVS != gotStatus.Valid {
				t.Fatalf("modules.Status = %+v, want status: %t", gotStatus, test.insertModuleBeforeMVS)
			}
			if int(gotStatus.Int64) != test.wantModulesStatus {
				t.Errorf("modules.status = %d, want %d", gotStatus.Int64, test.wantModulesStatus)
			}
		})
	}

}

func TestUpdateModuleVersionStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	mvs := &ModuleVersionStateForUpsert{
		ModulePath: "m.com",
		Version:    "v1.2.3",
		Timestamp:  time.Now(),
		Status:     200,
	}
	if err := testDB.UpsertModuleVersionState(ctx, mvs); err != nil {
		t.Fatal(err)
	}

	wantStatus := 999
	wantError := "Error"
	if err := testDB.UpdateModuleVersionStatus(ctx, mvs.ModulePath, mvs.Version, wantStatus, wantError); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetModuleVersionState(ctx, mvs.ModulePath, mvs.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != wantStatus || got.Error != wantError {
		t.Errorf("got %d, %q; want %d, %q", got.Status, got.Error, wantStatus, wantError)
	}
}

func TestHasGoMod(t *testing.T) {
	ptr := func(b bool) *bool { return &b }

	for _, test := range []struct {
		mvsHasRow          bool
		mvsValue           *bool
		modulesHasRow      bool
		modulesValue       bool
		want, wantNotFound bool
	}{
		{
			mvsHasRow: true,
			mvsValue:  ptr(false),
			want:      false,
		},
		{
			mvsHasRow: true,
			mvsValue:  ptr(true),
			want:      true,
		},
		{
			mvsHasRow:     true,
			mvsValue:      nil,
			modulesHasRow: false,
			wantNotFound:  true,
		},
		{
			mvsHasRow:     true,
			mvsValue:      nil,
			modulesHasRow: true,
			modulesValue:  true,
			want:          true,
		},
		{
			mvsHasRow:     true,
			mvsValue:      nil,
			modulesHasRow: true,
			modulesValue:  true,
			want:          true,
		},
		{
			mvsHasRow:    false,
			wantNotFound: true,
		},
	} {
		mvsValue := "NULL"
		if test.mvsValue != nil {
			mvsValue = fmt.Sprint(*test.mvsValue)
		}
		name := fmt.Sprintf("%t,%s,%t,%t", test.mvsHasRow, mvsValue, test.modulesHasRow, test.modulesValue)
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testDB, release := acquire(t)
			defer release()

			const (
				modulePath = "m.com"
				version    = "v1.2.3"
			)
			if test.mvsHasRow {
				_, err := testDB.db.Exec(ctx, `
						INSERT INTO module_version_states
						 (module_path, version, has_go_mod, index_timestamp, sort_version, incompatible, status)
						VALUES ($1, $2, $3, CURRENT_TIMESTAMP, '', false, 200)`,
					modulePath, version, test.mvsValue)
				if err != nil {
					t.Fatal(err)
				}
			}
			if test.modulesHasRow {
				m := sample.Module(modulePath, version, "")
				m.HasGoMod = test.modulesValue
				MustInsertModule(ctx, t, testDB, m)
			}

			got, err := testDB.HasGoMod(ctx, modulePath, version)
			if err != nil && !errors.Is(err, derrors.NotFound) {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("got %t, want %t", got, test.want)
			}
			if got := errors.Is(err, derrors.NotFound); got != test.wantNotFound {
				t.Errorf("not found: got %t, want %t", got, test.wantNotFound)
			}
		})
	}
}
