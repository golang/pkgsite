// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/postgres"
)

func TestExcluded(t *testing.T) {
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)

	// This test is intending to test PopulateExcluded not just IsExcluded.
	if err := PopulateExcluded(ctx, &config.Config{DynamicExcludeLocation: "testdata/excluded.txt"}, testDB); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path, version string
		want          bool
	}{
		{"github.com/golang/go", "", true},
		{"github.com/golang/pkgsite", "", false},
		{"bad.com/m", "v1.2.3", true},
		{"bad.com/m", "v1.2.4", false},
	} {
		got := testDB.IsExcluded(ctx, test.path, test.version)
		if got != test.want {
			t.Errorf("%q, %q: got %t, want %t", test.path, test.version, got, test.want)
		}
	}
}
