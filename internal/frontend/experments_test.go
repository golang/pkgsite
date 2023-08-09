// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/experiment"
)

func TestNewContextFromExps(t *testing.T) {
	for _, test := range []struct {
		mods []string
		want []string
	}{
		{
			mods: []string{"c", "a", "b"},
			want: []string{"a", "b", "c"},
		},
		{
			mods: []string{"d", "a"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			mods: []string{"d", "!b", "!a", "c"},
			want: []string{"c", "d"},
		},
	} {
		ctx := experiment.NewContext(context.Background(), "a", "b", "c")
		ctx = newContextFromExps(ctx, test.mods)
		got := experiment.FromContext(ctx).Active()
		sort.Strings(got)
		if !cmp.Equal(got, test.want) {
			t.Errorf("mods=%v:\ngot  %v\nwant %v", test.mods, got, test.want)
		}
	}
}
