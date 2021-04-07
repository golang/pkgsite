// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestStdlibPathForShortcut(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	m := sample.Module(stdlib.ModulePath, "v1.2.3",
		"encoding/json",                  // one match for "json"
		"text/template", "html/template", // two matches for "template"
	)
	ctx := context.Background()
	postgres.MustInsertModuleLatest(ctx, t, testDB, m)

	for _, test := range []struct {
		path string
		want string
	}{
		{"foo", ""},
		{"json", "encoding/json"},
		{"template", ""},
	} {
		got, err := stdlibPathForShortcut(ctx, testDB, test.path)
		if err != nil {
			t.Fatalf("%q: %v", test.path, err)
		}
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.path, got, test.want)
		}
	}
}
