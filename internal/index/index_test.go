// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
)

func TestGetVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allVersions := []*internal.IndexVersion{
		{Path: "github.com/my/module", Version: "v1.0.0"},
		{Path: "github.com/my/module", Version: "v1.1.0"},
		{Path: "github.com/my/module/v2", Version: "v2.0.0"},
	}

	for _, tc := range []struct {
		name     string
		limit    int
		versions []*internal.IndexVersion
		want     []*internal.IndexVersion
	}{
		{
			name:     "get all versions",
			limit:    10,
			versions: allVersions,
			want:     allVersions,
		}, {
			name:     "get partial versions",
			limit:    2,
			versions: allVersions,
			want:     allVersions[:2],
		}, {
			name:  "empty versions",
			limit: 10,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, teardown := SetupTestIndex(t, tc.versions)
			defer teardown()

			since := time.Time{}
			got, err := client.GetVersions(ctx, since, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("client.GetVersions(ctx, %q) mismatch (-want +got):\n%s", since, diff)
			}
		})
	}
}
