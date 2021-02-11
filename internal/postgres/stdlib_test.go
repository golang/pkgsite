// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetStdlibPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	// Insert two versions of some stdlib packages.
	for _, data := range []struct {
		version  string
		suffixes []string
	}{
		{
			// earlier version; should be ignored
			"v1.1.0",
			[]string{"bad/json"},
		},
		{
			"v1.2.0",
			[]string{
				"encoding/json",
				"archive/json",
				"net/http",     // no "json"
				"foo/json/moo", // "json" not the last component
				"bar/xjson",    // "json" not alone
				"baz/jsonx",    // ditto
			},
		},
	} {
		m := sample.Module(stdlib.ModulePath, data.version, data.suffixes...)
		for _, p := range m.Packages() {
			p.Imports = nil
		}
		MustInsertModule(ctx, t, testDB, m)
	}

	got, err := testDB.GetStdlibPathsWithSuffix(ctx, "json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"archive/json", "encoding/json"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
