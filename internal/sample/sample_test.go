// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sample

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
)

func TestPackageSampling(t *testing.T) {
	sampler := PackageSampler(func() *internal.Package {
		return &internal.Package{}
	})
	got := sampler.Sample(
		WithName("bar"),
		WithPath("test.module/bar"),
		WithImports("test.module/baz"),
	)
	want := &internal.Package{
		Name:    "bar",
		Path:    "test.module/bar",
		Imports: []string{"test.module/baz"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sample mismatch (-want +got):\n%s", diff)
	}
}

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
	want := &internal.Version{
		VersionInfo: internal.VersionInfo{
			ModulePath:  "test.module/v2",
			Version:     "v2.0.0-alpha.1",
			VersionType: internal.VersionTypePrerelease,
		},
		Packages: []*internal.Package{
			Package(WithPath("test.module/v2"), WithV1Path("test.module")),
			Package(WithPath("test.module/v2/foo"), WithV1Path("test.module/foo")),
			Package(WithPath("test.module/v2/bar"), WithV1Path("test.module/bar")),
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sample mismatch (-want, +got):\n%s", diff)
	}
}
