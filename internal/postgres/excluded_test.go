// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"
)

func TestIsExcluded(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	for _, pat := range []string{"bad", "badslash/", "baddy@v1.2.3", "github.com/bad", "exact:github.com/CASE/go", "exact:github.com/PACKAGE@v1.0.0"} {
		if err := testDB.InsertExcludedPattern(ctx, pat, "someone", "because"); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		path    string
		version string
		want    bool
	}{
		{"fine", "", false},
		{"ba", "", false},
		{"bad", "", true},
		{"badness", "", false},
		{"bad/ness", "", true},
		{"bad.com/foo", "", false},
		{"badslash", "", true},
		{"badslash/more", "", true},
		{"badslash/more", "v1.2.3", true},
		{"baddys", "v1.2.3", false},
		{"baddy", "v1.2.4", false},
		{"baddy", "", false},
		{"baddy", "v1.2.3", true},

		// tests for case insensitivity
		{"Bad", "", true},
		{"Bad/repo", "", true},
		{"baDDy", "v1.2.3", true},
		{"baDDy", "v1.2.4", false},
		{"github.com/Bad", "", true},
		{"github.com/bad/repo", "", true},
		{"github.com/bad/Repo", "", true},
		{"github.com/Bad/repo", "", true},

		// tests for exact: prefix
		{"github.com/CASE/go", "", true},
		{"github.com/case/go", "", false},
		{"github.com/CASE/go/sub", "", true},
		{"github.com/case/go/sub", "", false},
		{"github.com/PACKAGE", "v1.0.0", true},
		{"github.com/package", "v1.0.0", false},
		{"github.com/PACKAGE", "v1.0.1", false},
	} {
		got := testDB.IsExcluded(ctx, test.path, test.version)
		if got != test.want {
			t.Errorf("%q: got %t, want %t", test.path, got, test.want)
		}
	}
}
