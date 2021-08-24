// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
)

func TestCache(t *testing.T) {
	ds := newDataSource(nil, nil)
	m1 := &internal.Module{}
	ds.cachePut("m1", fetch.LocalVersion, m1, nil)
	ds.cachePut("m2", "v1.0.0", nil, derrors.NotFound)

	for _, test := range []struct {
		path, version string
		wantm         *internal.Module
		wante         error
	}{
		{"m1", fetch.LocalVersion, m1, nil},
		{"m1", "v1.2.3", m1, nil}, // find m1 under LocalVersion
		{"m2", "v1.0.0", nil, derrors.NotFound},
		{"m3", "v1.0.0", nil, nil},
	} {
		gotm, gote := ds.cacheGet(test.path, test.version)
		if gotm != test.wantm || gote != test.wante {
			t.Errorf("%s@%s: got (%v, %v), want (%v, %v)", test.path, test.version, gotm, gote, test.wantm, test.wante)
		}
	}
}
