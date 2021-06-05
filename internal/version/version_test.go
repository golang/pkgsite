// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"testing"

	"golang.org/x/mod/semver"
)

func TestForSorting(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"v1.2.3", "1,2,3~"},
		{"v12.48.301", "a12,a48,b301~"},
		{"v0.9.3-alpha.1", "0,9,3,~alpha,1"},
		{"v1.2.3-rc.20150901.-", "1,2,3,~rc,g20150901,~-"},
		{"v1.2.3-alpha.789+build", "1,2,3,~alpha,b789"},
	} {
		got := ForSorting(test.in)
		if got != test.want {
			t.Errorf("ForSorting(%s) = %s, want %s", test.in, got, test.want)
		}
	}
}

func TestForSortingOrder(t *testing.T) {
	// A list of valid semantic versions, in order.
	semvers := []string{
		"v0.0.0-20180713131340-b395d2d6f5ee",
		"v0.0.0-20190124233150-8f7fa2680c82",
		"v0.0.0",
		"v0.0.1",
		"v0.1.0",
		"v1.0.0-alpha",
		"v1.0.0-alpha.1",
		"v1.0.0-alpha.beta",
		"v1.0.0-beta",
		"v1.0.0-beta.2",
		"v1.0.0-beta.11",
		"v1.0.0-rc.1",
		"v1.0.0",
		"v1.2.0",
		"v1.11.0",
		"v1.12.0-alph.1",
		"v1.12.0-alpha",
		// These next two would order incorrectly if we used '.' as a separator, because
		// '.' comes after '-'.
		"v2.0.0-z.a",
		"v2.0.0-z-",
		// These next two would sort incorrectly if we did not prepend non-numeric components
		// with a '~'.
		"v2.1.0-a.1",
		"v2.1.0-a.-",
	}

	// Check that the test has the ordering right according to the semver package.
	for i := range semvers {
		if !semver.IsValid(semvers[i]) {
			t.Fatalf("test is broken: bad semver: %s", semvers[i])
		}
		if i > 0 {
			if semver.Compare(semvers[i-1], semvers[i]) >= 0 {
				t.Fatalf("test is broken: %s is not less than %s", semvers[i-1], semvers[i])
			}
		}
	}

	// Check that ForSorting produces the correct ordering.
	var prev string
	for _, v := range semvers {
		got := ForSorting(v)
		if prev != "" && prev >= got {
			t.Errorf("%s: %s >= %s, want less than", v, prev, got)
		}
		prev = got
	}
}

func TestAppendNumericPrefix(t *testing.T) {
	for _, test := range []struct {
		n    int
		want string
	}{
		{1, ""},
		{2, "a"},
		{3, "b"},
		{26, "y"},
		{53, "zz"},    // 53-1 = 26*2
		{100, "zzzu"}, // 100-1 = 26*3 + 21
	} {
		got := string(appendNumericPrefix(nil, test.n))
		if got != test.want {
			t.Errorf("%d: got %s, want %s", test.n, got, test.want)
		}
	}
}

func TestParseVersionType(t *testing.T) {
	testCases := []struct {
		name, version   string
		wantVersionType Type
		wantErr         bool
	}{
		{
			name:            "pseudo major version",
			version:         "v1.0.0-20190311183353-d8887717615a",
			wantVersionType: TypePseudo,
		},
		{
			name:            "pseudo prerelease version",
			version:         "v1.2.3-pre.0.20190311183353-d8887717615a",
			wantVersionType: TypePseudo,
		},
		{
			name:            "pseudo minor version",
			version:         "v1.2.4-0.20190311183353-d8887717615a",
			wantVersionType: TypePseudo,
		},
		{
			name:            "pseudo version invalid",
			version:         "v1.2.3-20190311183353-d8887717615a",
			wantVersionType: TypePrerelease,
		},
		{
			name:            "valid release",
			version:         "v1.0.0",
			wantVersionType: TypeRelease,
		},
		{
			name:            "valid prerelease",
			version:         "v1.0.0-alpha.1",
			wantVersionType: TypePrerelease,
		},
		{
			name:            "invalid version",
			version:         "not_a_version",
			wantVersionType: "",
			wantErr:         true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if gotVt, err := ParseType(test.version); (test.wantErr == (err != nil)) && test.wantVersionType != gotVt {
				t.Errorf("parseVersionType(%v) = %v, want %v", test.version, gotVt, test.wantVersionType)
			}
		})
	}
}

func TestLatestOf(t *testing.T) {
	for _, test := range []struct {
		name     string
		versions []string
		want     string
	}{
		{
			name:     "highest release",
			versions: []string{"v1.2.3", "v1.0.0", "v1.9.0-pre"},
			want:     "v1.2.3",
		},
		{
			name:     "highest pre-release if no release",
			versions: []string{"v1.2.3-alpha", "v1.0.0-beta", "v1.9.0-pre"},
			want:     "v1.9.0-pre",
		},
		{
			name:     "prefer pre-release to pseudo",
			versions: []string{"v1.0.0-20180713131340-b395d2d6f5ee", "v0.0.0-alpha"},
			want:     "v0.0.0-alpha",
		},

		{
			name:     "highest pseudo if no pre-release or release",
			versions: []string{"v0.0.0-20180713131340-b395d2d6f5ee", "v0.0.0-20190124233150-8f7fa2680c82"},
			want:     "v0.0.0-20190124233150-8f7fa2680c82",
		},
		{
			name:     "use incompatible",
			versions: []string{"v1.2.3", "v1.0.0", "v2.0.0+incompatible"},
			want:     "v2.0.0+incompatible",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := LatestOf(test.versions)
			if got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestLatest(t *testing.T) {
	pseudo := "v0.0.0-20190124233150-8f7fa2680c82"
	for _, test := range []struct {
		name     string
		versions []string
		hasGoMod func(string) (bool, error)
		want     string
	}{
		{
			name:     "empty",
			versions: nil,
			want:     "",
		},
		{
			name:     "tagged release",
			versions: []string{pseudo, "v0.1.0", "v1.2.3-pre"},
			want:     "v0.1.0",
		},
		{
			name:     "tagged pre-release",
			versions: []string{pseudo, "v1.2.3-pre"},
			want:     "v1.2.3-pre",
		},
		{
			name:     "pseudo",
			versions: []string{pseudo},
			want:     pseudo,
		},
		{
			name:     "incompatible with go.mod",
			versions: []string{"v2.0.0+incompatible", "v1.2.3"},
			want:     "v1.2.3",
		},
		{
			name:     "incompatible without go.mod",
			versions: []string{"v2.0.0+incompatible", "v1.2.3"},
			hasGoMod: func(v string) (bool, error) { return false, nil },
			want:     "v2.0.0+incompatible",
		},
		{
			name: "incompatible without tagged go.mod",
			// Although the latest compatible version has a go.mod file,
			// it is not a tagged version.
			versions: []string{pseudo, "v2.0.0+incompatible"},
			want:     "v2.0.0+incompatible",
		},
	} {
		t.Run(test.name, func(t *testing.T) {

			if test.hasGoMod == nil {
				test.hasGoMod = func(v string) (bool, error) { return true, nil }
			}
			got, err := LatestVersion(test.versions, test.hasGoMod)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
