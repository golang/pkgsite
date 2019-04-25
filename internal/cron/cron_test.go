// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_cron_test", m, &testDB)
}

func setupIndex(t *testing.T, versions []map[string]string) (func(t *testing.T), *httptest.Server) {
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, v := range versions {
			json.NewEncoder(w).Encode(v)
		}
	}))

	fn := func(t *testing.T) {
		index.Close()
	}
	return fn, index
}

// versionLogArrayToString outputs a string for an array for version logs. It
// is used for testing in printing errors.
func versionLogArrayToString(logs []*internal.VersionLog) string {
	var b strings.Builder
	for _, l := range logs {
		fmt.Fprintf(&b, "%+v\n", l)
	}
	return b.String()
}

func TestGetVersionsFromIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, tc := range []struct {
		name      string
		indexInfo []map[string]string
		wantLogs  []*internal.VersionLog
	}{
		{
			name: "valid_get_versions",
			indexInfo: []map[string]string{
				map[string]string{
					"path":    "my.mod/module",
					"version": "v1.0.0",
				},
				map[string]string{
					"path":    "my.mod/module",
					"version": "v1.1.0",
				},
				map[string]string{
					"path":    "my.mod/module/v2",
					"version": "v2.0.0",
				},
			},
		},
		{
			name:      "empty_get_versions",
			indexInfo: []map[string]string{},
		},
	} {
		wantLogs := []*internal.VersionLog{}
		for _, v := range tc.indexInfo {
			wantLogs = append(wantLogs, &internal.VersionLog{
				ModulePath: v["path"],
				Version:    v["version"],
				Source:     internal.VersionSourceProxyIndex,
			})
		}
		if len(wantLogs) > 0 {
			tc.wantLogs = wantLogs
		}

		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, index := setupIndex(t, tc.indexInfo)
			defer teardownTestCase(t)
			logs, err := getVersionsFromIndex(ctx, index.URL, time.Time{})
			if err != nil {
				t.Fatalf("getVersionFromIndex(ctx, %q, %q) error: %v",
					index.URL, time.Time{}, err)
			}

			for _, l := range logs {
				l.CreatedAt = time.Time{}
			}

			if !reflect.DeepEqual(logs, tc.wantLogs) {
				t.Errorf("getVersionFromIndex(ctx, %q, %q) = \n %v; want = \n %v", index.URL, time.Time{}.String(), versionLogArrayToString(logs), versionLogArrayToString(tc.wantLogs))
			}
		})
	}
}

func TestFetchAndStoreVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, tc := range []struct {
		name            string
		indexInfo       []map[string]string
		oldVersionLogs  []*internal.VersionLog
		wantVersionLogs []*internal.VersionLog
	}{
		{
			name: "version-logs-no-existing-entries",
			indexInfo: []map[string]string{
				map[string]string{
					"path":    "my.mod/module",
					"version": "v1.0.0",
				},
			},
			oldVersionLogs: nil,
			wantVersionLogs: []*internal.VersionLog{
				&internal.VersionLog{
					ModulePath: "my.mod/module",
					Version:    "v1.0.0",
					Source:     internal.VersionSourceProxyIndex,
				},
			},
		},
		{
			name: "version-logs-existing-duplicate-entry",
			indexInfo: []map[string]string{
				map[string]string{
					"path":    "my.mod/module",
					"version": "v1.0.0",
				},
				map[string]string{
					"path":    "my.mod/module",
					"version": "v2.0.0",
				},
			},
			oldVersionLogs: []*internal.VersionLog{
				&internal.VersionLog{
					ModulePath: "my.mod/module",
					Version:    "v1.0.0",
					Source:     internal.VersionSourceProxyIndex,
				},
			},
			wantVersionLogs: []*internal.VersionLog{
				&internal.VersionLog{
					ModulePath: "my.mod/module",
					Version:    "v1.0.0",
					Source:     internal.VersionSourceProxyIndex,
				},
				&internal.VersionLog{
					ModulePath: "my.mod/module",
					Version:    "v2.0.0",
					Source:     internal.VersionSourceProxyIndex,
				},
			},
		},
		{
			name:            "version-logs-no-new-entries",
			indexInfo:       []map[string]string{},
			oldVersionLogs:  nil,
			wantVersionLogs: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, index := setupIndex(t, tc.indexInfo)
			defer teardownTestCase(t)

			defer postgres.ResetTestDB(testDB, t)

			if err := testDB.InsertVersionLogs(ctx, tc.oldVersionLogs); err != nil {
				t.Fatalf("db.InsertVersionLogs(ctx, %v): %v", tc.oldVersionLogs, err)
			}

			got, err := FetchAndStoreVersions(ctx, index.URL, testDB)
			if err != nil {
				t.Fatalf("FetchAndStoreVersions(ctx, %q, %v): %v", index.URL, testDB, err)
			}

			// do not compare the timestamps, since they are set inside
			// FetchAndStoreVersions.
			for _, l := range got {
				l.CreatedAt = time.Time{}
			}

			if !reflect.DeepEqual(got, tc.wantVersionLogs) {
				t.Fatalf("FetchAndStoreVersions(ctx, %q, %v) = %v; want %v",
					index.URL, testDB, versionLogArrayToString(got), versionLogArrayToString(tc.wantVersionLogs))
			}
		})
	}
}
