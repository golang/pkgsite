// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetLatestMajorPathForV1Path(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	checkLatest := func(t *testing.T, versions []string, v1path string, version, suffix string) {
		t.Helper()
		gotPath, gotVer, err := testDB.GetLatestMajorPathForV1Path(ctx, v1path)
		if err != nil {
			t.Fatal(err)
		}
		want := sample.ModulePath
		if suffix != "" {
			want = want + "/" + suffix
		}
		var wantVer int
		if version == "" {
			wantVer = 1
		} else {
			wantVer, err = strconv.Atoi(strings.TrimPrefix(version, "v"))
			if err != nil {
				t.Fatal(err)
			}
		}
		if gotPath != want || gotVer != wantVer {
			t.Errorf("GetLatestMajorPathForV1Path(%q) = %q, %d, want %q, %d", v1path, gotPath, gotVer, want, wantVer)
		}
	}

	for _, test := range []struct {
		name, want string
		versions   []string
	}{
		{
			"want highest major version",
			"v11",
			[]string{"", "v2", "v11"},
		},
		{
			"only v1 version",
			"",
			[]string{""},
		},
		{
			"no v1 version",
			"v4",
			[]string{"v4"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ResetTestDB(testDB, t)
			suffix := "a/b/c"

			for _, v := range test.versions {
				modpath := sample.ModulePath
				if v != "" {
					modpath = modpath + "/" + v
				}
				if v == "" {
					v = sample.VersionString
				} else {
					v = v + ".0.0"
				}
				m := sample.Module(modpath, v, suffix)
				if err := testDB.InsertModule(ctx, m); err != nil {
					t.Fatal(err)
				}
			}
			t.Run("module", func(t *testing.T) {
				v1path := sample.ModulePath
				checkLatest(t, test.versions, v1path, test.want, test.want)
			})
			t.Run("package", func(t *testing.T) {
				want := test.want
				if test.want != "" {
					want += "/"
				}
				v1path := sample.ModulePath + "/" + suffix
				checkLatest(t, test.versions, v1path, test.want, want+suffix)
			})
		})
	}
}
