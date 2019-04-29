// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_cron_test", m, &testDB)
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
			teardownTestCase, client := index.SetupTestIndex(t, tc.indexInfo)
			defer teardownTestCase(t)

			defer postgres.ResetTestDB(testDB, t)

			if err := testDB.InsertVersionLogs(ctx, tc.oldVersionLogs); err != nil {
				t.Fatalf("db.InsertVersionLogs(ctx, %v): %v", tc.oldVersionLogs, err)
			}

			got, err := FetchAndStoreVersions(ctx, client, testDB)
			if err != nil {
				t.Fatalf("FetchAndStoreVersions(ctx, %v, %v): %v", client, testDB, err)
			}

			if diff := cmp.Diff(tc.wantVersionLogs, got); diff != "" {
				t.Errorf("FetchAndStoreVersions(ctx, %v, %v) mismatch (-want +got):\n%s", client, testDB, diff)
			}
		})
	}
}
