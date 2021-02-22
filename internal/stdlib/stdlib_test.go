// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdlib

import (
	"io/ioutil"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/version"
)

func TestTagForVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "std version v1.0.0",
			version: "v1.0.0",
			want:    "go1",
		},
		{
			name:    "std version v1.12.5",
			version: "v1.12.5",
			want:    "go1.12.5",
		},
		{
			name:    "std version v1.13, incomplete canonical version",
			version: "v1.13",
			want:    "go1.13",
		},
		{
			name:    "std version v1.13.0-beta.1",
			version: "v1.13.0-beta.1",
			want:    "go1.13beta1",
		},
		{
			name:    "std version v1.9.0-rc.2",
			version: "v1.9.0-rc.2",
			want:    "go1.9rc2",
		},
		{
			name:    "std with digitless prerelease",
			version: "v1.13.0-prerelease",
			want:    "go1.13prerelease",
		},
		{
			name:    "version v1.13.0",
			version: "v1.13.0",
			want:    "go1.13",
		},
		{
			name:    "master branch",
			version: "master",
			want:    "master",
		},
		{
			name:    "master version",
			version: TestVersion,
			want:    "master",
		},
		{
			name:    "bad std semver",
			version: "v1.x",
			wantErr: true,
		},
		{
			name:    "more bad std semver",
			version: "v1.0-",
			wantErr: true,
		},
		{
			name:    "bad prerelease",
			version: "v1.13.0-beta1",
			wantErr: true,
		},
		{
			name:    "another bad prerelease",
			version: "v1.13.0-whatevs99",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := TagForVersion(test.version)
			if (err != nil) != test.wantErr {
				t.Errorf("TagForVersion(%q) = %q, %v, wantErr %v", test.version, got, err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("TagForVersion(%q) = %q, %v, wanted %q, %v", test.version, got, err, test.want, nil)
			}
		})
	}
}

func TestMajorVersionForVersion(t *testing.T) {
	for _, test := range []struct {
		in   string
		want string // empty => error
	}{
		{"", ""},
		{"garbage", ""},
		{"v1.0.0", "go1"},
		{"v1.13.3", "go1"},
		{"v1.9.0-rc.2", "go1"},
		{"v2.1.3", "go2"},
	} {
		got, err := MajorVersionForVersion(test.in)
		if (err != nil) != (test.want == "") {
			t.Errorf("%q: err: got %v, wanted error: %t", test.in, err, test.want == "")
		}
		if err == nil && got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}
}

func TestZip(t *testing.T) {
	UseTestData = true
	defer func() { UseTestData = false }()
	for _, resolvedVersion := range []string{"v1.14.6", "v1.12.5", "v1.3.2", TestVersion} {
		t.Run(resolvedVersion, func(t *testing.T) {
			zr, gotResolvedVersion, gotTime, err := Zip(resolvedVersion)
			if err != nil {
				t.Fatal(err)
			}
			if resolvedVersion == "master" {
				if !version.IsPseudo(gotResolvedVersion) {
					t.Errorf("resolved version: %s is not a pseudo-version", gotResolvedVersion)
				}
			} else if gotResolvedVersion != resolvedVersion {
				t.Errorf("resolved version: got %s, want %s", gotResolvedVersion, resolvedVersion)
			}
			if !gotTime.Equal(TestCommitTime) {
				t.Errorf("commit time: got %s, want %s", gotTime, TestCommitTime)
			}
			wantFiles := map[string]bool{
				"LICENSE":               true,
				"errors/errors.go":      true,
				"errors/errors_test.go": true,
			}
			if semver.Compare(resolvedVersion, "v1.4.0") > 0 || resolvedVersion == TestVersion {
				wantFiles["README.md"] = true
			} else {
				wantFiles["README"] = true
			}
			if semver.Compare(resolvedVersion, "v1.13.0") > 0 || resolvedVersion == TestVersion {
				wantFiles["cmd/README.vendor"] = true
			}

			wantPrefix := "std@" + resolvedVersion + "/"
			readmeVendorFile := wantPrefix + "README.vendor"
			for _, f := range zr.File {
				if f.Name == readmeVendorFile {
					t.Fatalf("got %q; want file to be removed", readmeVendorFile)
				}
				if !strings.HasPrefix(f.Name, wantPrefix) {
					t.Errorf("filename %q missing prefix %q", f.Name, wantPrefix)
					continue
				}
				delete(wantFiles, f.Name[len(wantPrefix):])
			}
			if len(wantFiles) > 0 {
				t.Errorf("zip missing files: %v", reflect.ValueOf(wantFiles).MapKeys())
			}
			for _, f := range zr.File {
				if f.Name == wantPrefix+"go.mod" {
					r, err := f.Open()
					if err != nil {
						t.Fatal(err)
					}
					defer r.Close()
					b, err := ioutil.ReadAll(r)
					if err != nil {
						t.Fatal(err)
					}
					if got, want := string(b), "module std\n"; got != want {
						t.Errorf("go.mod: got %q, want %q", got, want)
					}
					break
				}
			}
		})
	}
}

func TestZipInfo(t *testing.T) {
	UseTestData = true
	defer func() { UseTestData = false }()

	for _, tc := range []struct {
		requestedVersion string
		want             string
	}{
		{
			requestedVersion: "latest",
			want:             "v1.14.6",
		},
		{
			requestedVersion: "master",
			want:             "master",
		},
	} {
		gotVersion, err := ZipInfo(tc.requestedVersion)
		if err != nil {
			t.Fatal(err)
		}
		if want := tc.want; gotVersion != want {
			t.Errorf("version: got %q, want %q", gotVersion, want)
		}
	}
}

func TestVersions(t *testing.T) {
	UseTestData = true
	defer func() { UseTestData = false }()

	got, err := Versions()
	if err != nil {
		t.Fatal(err)
	}
	gotmap := map[string]bool{}
	for _, g := range got {
		gotmap[g] = true
	}
	wants := []string{
		"v1.4.2",
		"v1.9.0-rc.1",
		"v1.11.0",
		"v1.13.0-beta.1",
	}
	for _, w := range wants {
		if !gotmap[w] {
			t.Errorf("missing %s", w)
		}
	}
}

func TestVersionForTag(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"", ""},
		{"go1", "v1.0.0"},
		{"go1.9beta2", "v1.9.0-beta.2"},
		{"go1.12", "v1.12.0"},
		{"go1.9.7", "v1.9.7"},
		{"go2.0", "v2.0.0"},
		{"go1.9rc2", "v1.9.0-rc.2"},
		{"go1.1beta", ""},
		{"go1.0", ""},
		{"weekly.2012-02-14", ""},
		{"latest", "latest"},
	} {
		got := VersionForTag(test.in)
		if got != test.want {
			t.Errorf("VersionForTag(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestContains(t *testing.T) {
	for _, test := range []struct {
		in   string
		want bool
	}{
		{"fmt", true},
		{"encoding/json", true},
		{"something/with.dots", true},
		{"example.com", false},
		{"example.com/fmt", false},
	} {
		got := Contains(test.in)
		if got != test.want {
			t.Errorf("Contains(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestDirectory(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    string
	}{
		{
			version: "v1.3.0-beta2",
			want:    "src/pkg",
		},
		{
			version: "v1.16.0-beta1",
			want:    "src",
		},
		{
			version: "master",
			want:    "src",
		},
	} {
		got := Directory(tc.version)
		if got != tc.want {
			t.Errorf("Directory(%s) = %s, want %s", tc.version, got, tc.want)
		}
	}
}
