// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetImportedByCount(t *testing.T) {
	fds := fakedatasource.New()

	newModule := func(modPath string, imports []string, numImportedBy int) *internal.Module {
		m := sample.Module(modPath, sample.VersionString, "")
		m.Packages()[0].Imports = imports
		m.Packages()[0].NumImportedBy = numImportedBy
		return m
	}

	p1 := "path.to/foo"
	p2 := "path2.to/foo"
	p3 := "path3.to/foo"
	mod1 := newModule(p1, nil, 2)
	mod2 := newModule(p2, []string{p1}, 1)
	mod3 := newModule(p3, []string{p1, p2}, 0)
	ctx := context.Background()
	for _, m := range []*internal.Module{mod1, mod2, mod3} {
		fds.MustInsertModule(ctx, m)
	}

	for _, test := range []struct {
		mod  *internal.Module
		want int
	}{
		{
			mod:  mod3,
			want: 0,
		},
		{
			mod:  mod2,
			want: 1,
		},
		{
			mod:  mod1,
			want: 2,
		},
	} {
		pkg := test.mod.Packages()[0]
		t.Run(test.mod.ModulePath, func(t *testing.T) {
			got := pkg.NumImportedBy
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("getImportedByCount(ctx, db, %q) mismatch (-want +got):\n%s", pkg.Path, diff)
			}
		})
	}
}
