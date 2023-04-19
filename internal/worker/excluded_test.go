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
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	// This test is intending to test PopulateExcluded not just IsExcluded.
	if err := PopulateExcluded(ctx, &config.Config{DynamicExcludeLocation: "testdata/excluded.txt"}, testDB); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path string
		want bool
	}{
		{"github.com/golang/go", true},
		{"github.com/golang/pkgsite", false},
	} {
		got, err := testDB.IsExcluded(ctx, test.path)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("%q: got %t, want %t", test.path, got, test.want)
		}
	}
}
