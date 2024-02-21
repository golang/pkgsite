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

	for _, pat := range []string{"bad", "badslash/", "baddy@v1.2.3"} {
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
		{"badslash", "", false},
		{"badslash/more", "", true},
		{"badslash/more", "v1.2.3", true},
		{"baddys", "v1.2.3", false},
		{"baddy", "v1.2.4", false},
		{"baddy", "", false},
		{"baddy", "v1.2.3", true},
	} {
		got := testDB.IsExcluded(ctx, test.path, test.version)
		if got != test.want {
			t.Errorf("%q: got %t, want %t", test.path, got, test.want)
		}
	}
}
