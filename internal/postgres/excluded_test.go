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

	if err := testDB.InsertExcludedPrefix(ctx, "bad", "someone", "because"); err != nil {
		t.Fatal(err)
	}
	if err := testDB.InsertExcludedPrefix(ctx, "badslash/", "someone", "because"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		want bool
	}{
		{"fine", false},
		{"ba", false},
		{"bad", true},
		{"badness", false},
		{"bad/ness", true},
		{"bad.com/foo", false},
		{"badslash", false},
		{"badslash/more", true},
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
