// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sample

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
)

func TestVersionSampling(t *testing.T) {
	sampler := VersionSampler(func() *internal.Version {
		return &internal.Version{}
	})
	got := sampler.Sample(
		WithModulePath("test.module/v2"),
		WithVersion("v2.0.0-alpha.1"),
		WithVersionType(internal.VersionTypePrerelease),
		WithSuffixes("", "foo", "bar"),
	)
	pkg := func(path, v1path string) *internal.Package {
		p := Package()
		p.Path = path
		p.V1Path = v1path
		return p
	}

	want := &internal.Version{
		VersionInfo: internal.VersionInfo{
			ModulePath:  "test.module/v2",
			Version:     "v2.0.0-alpha.1",
			VersionType: internal.VersionTypePrerelease,
		},
		Packages: []*internal.Package{
			pkg("test.module/v2", "test.module"),
			pkg("test.module/v2/foo", "test.module/foo"),
			pkg("test.module/v2/bar", "test.module/bar"),
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sample mismatch (-want, +got):\n%s", diff)
	}
}
