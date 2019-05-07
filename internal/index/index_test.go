// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
)

func TestGetVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tc := range []struct {
		name      string
		indexInfo []*internal.IndexVersion
	}{
		{
			name: "valid_get_versions",
			indexInfo: []*internal.IndexVersion{
				{Path: "my.mod/module", Version: "v1.0.0"},
				{Path: "my.mod/module", Version: "v1.1.0"},
				{Path: "my.mod/module/v2", Version: "v2.0.0"},
			},
		}, {
			name:      "empty_get_versions",
			indexInfo: []*internal.IndexVersion{},
		},
	} {
		var wantLogs []*internal.VersionLog
		for _, v := range tc.indexInfo {
			wantLogs = append(wantLogs, &internal.VersionLog{
				ModulePath: v.Path,
				Version:    v.Version,
				Source:     internal.VersionSourceProxyIndex,
			})
		}

		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, client := SetupTestIndex(t, tc.indexInfo)
			defer teardownTestCase(t)

			since := time.Time{}
			got, err := client.GetVersions(ctx, since)
			if err != nil {
				t.Fatalf("client.GetVersions(ctx, %q) error: %v", since, err)
			}

			if diff := cmp.Diff(wantLogs, got); diff != "" {
				t.Errorf("client.GetVersions(ctx, %q) mismatch (-want +got):\n%s", since, diff)
			}
		})
	}
}
