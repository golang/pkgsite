// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdlib

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"reflect"
	"testing"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/testenv"
	"golang.org/x/pkgsite/internal/version"
)

var (
	clone    = flag.Bool("clone", false, "test actual clones of the Go repo")
	repoPath = flag.String("path", "", "path to Go repo to test")
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
			name:    "version v1.20.0-rc.2",
			version: "v1.20.0-rc.2",
			want:    "go1.20rc2",
		},
		{
			name:    "version v1.20.0",
			version: "v1.20.0",
			want:    "go1.20",
		},
		{
			name:    "version v1.21.0-rc.2",
			version: "v1.21.0-rc.2",
			want:    "go1.21rc2",
		},
		{
			name:    "version v1.21.0",
			version: "v1.21.0",
			want:    "go1.21.0",
		},
		{
			name:    "master branch",
			version: "master",
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
				t.Fatalf("TagForVersion(%q) = %q, %v, wanted %q, %v", test.version, got, err, test.want, nil)
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
		{"v2.1.3", "go2"},
		{"v0.0.0-20230307225218-457fd1d52d17", "go1"},
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

func TestContentDir(t *testing.T) {
	ctx := context.Background()
	testenv.MustHaveExecPath(t, "git")
	defer WithTestData()()
	for _, resolvedVersion := range []string{
		"v1.3.2",
		"v1.12.5",
		"v1.14.6",
		DevFuzz,
		version.Master,
	} {
		t.Run(resolvedVersion, func(t *testing.T) {
			cdir, gotResolvedVersion, gotTime, err := ContentDir(ctx, resolvedVersion)
			if err != nil {
				t.Fatal(err)
			}
			if SupportedBranches[resolvedVersion] {
				if !version.IsPseudo(gotResolvedVersion) {
					t.Errorf("resolved version: %s is not a pseudo-version", gotResolvedVersion)
				}
			} else if gotResolvedVersion != resolvedVersion {
				t.Errorf("resolved version: got %s, want %s", gotResolvedVersion, resolvedVersion)
			}
			if !gotTime.Equal(TestCommitTime) {
				t.Errorf("commit time: got %s, want %s", gotTime, TestCommitTime)
			}
			checkContentDirFiles(t, cdir, resolvedVersion)
		})
	}
}

func TestContentDirCloneAndOpen(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T) {
		for _, resolvedVersion := range []string{
			"v1.3.2",
			"v1.14.6",
			version.Master,
			version.Latest,
		} {
			t.Run(resolvedVersion, func(t *testing.T) {
				cdir, _, _, err := ContentDir(ctx, resolvedVersion)
				if err != nil {
					t.Fatal(err)
				}
				checkContentDirFiles(t, cdir, resolvedVersion)
			})
		}
	}

	t.Run("clone", func(t *testing.T) {
		if !*clone {
			t.Skip("-clone not supplied")
		}
		defer withGoRepo(&remoteGoRepo{})()
		run(t)
	})
	t.Run("local", func(t *testing.T) {
		if *repoPath == "" {
			t.Skip("-path not supplied")
		}
		lgr := newLocalGoRepo(*repoPath)

		defer withGoRepo(lgr)()
		run(t)
	})
}

func checkContentDirFiles(t *testing.T, cdir fs.FS, resolvedVersion string) {
	wantFiles := map[string]bool{
		"LICENSE":               true,
		"errors/errors.go":      true,
		"errors/errors_test.go": true,
	}
	if semver.Compare(resolvedVersion, "v1.13.0") > 0 || resolvedVersion == TestMasterVersion {
		wantFiles["cmd/README.vendor"] = true
	}
	if semver.Compare(resolvedVersion, "v1.14.0") > 0 {
		wantFiles["context/context.go"] = true
	}
	const readmeVendorFile = "README.vendor"
	if _, err := fs.Stat(cdir, readmeVendorFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("fs.Stat returned %v; want %q to be removed", err, readmeVendorFile)
	}
	err := fs.WalkDir(cdir, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		delete(wantFiles, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wantFiles) > 0 {
		t.Errorf("zip missing files: %v", reflect.ValueOf(wantFiles).MapKeys())
	}
}

func TestZipInfo(t *testing.T) {
	defer WithTestData()()

	for _, tc := range []struct {
		requestedVersion string
		want             string
	}{
		{
			requestedVersion: "latest",
			want:             "v1.21.0",
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
	testVersions := func(wants []string) {
		got, err := Versions()
		if err != nil {
			t.Fatal(err)
		}
		gotmap := map[string]bool{}
		for _, g := range got {
			gotmap[g] = true
		}
		for _, w := range wants {
			if !gotmap[w] {
				t.Errorf("missing %s", w)
			}
		}
	}

	commonWants := []string{
		"v1.4.2",
		"v1.9.0-rc.1",
		"v1.11.0",
		"v1.13.0-beta.1",
	}
	otherWants := append([]string{"v1.17.6"}, commonWants...)
	t.Run("test", func(t *testing.T) {
		defer WithTestData()()
		testWants := append([]string{"v1.21.0"}, commonWants...)
		testVersions(testWants)
	})
	t.Run("local", func(t *testing.T) {
		if *repoPath == "" {
			t.Skip("-path not supplied")
		}
		lgr := newLocalGoRepo(*repoPath)

		defer withGoRepo(lgr)()
		testVersions(otherWants)
	})
	t.Run("remote", func(t *testing.T) {
		if !*clone {
			t.Skip("-clone not supplied")
		}
		defer withGoRepo(&remoteGoRepo{})()
		testVersions(otherWants)
	})
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
		{"go1.21.0", "v1.21.0"},
		{"go1.21", "v1.21.0"},
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

func TestVersionMatchesHash(t *testing.T) {
	v := "v0.0.0-20210910212848-c8dfa306babb"
	h := "c8dfa306babb91e88f8ba25329b3ef8aa11944e1"
	if !VersionMatchesHash(v, h) {
		t.Error("got false, want true")
	}
	h = "c8dfa306babXb91e88f8ba25329b3ef8aa11944e1"
	if VersionMatchesHash(v, h) {
		t.Error("got true, want false")
	}
}

func TestResolveSupportedBranches(t *testing.T) {
	testenv.MustHaveExternalNetwork(t) // ResolveSupportedBranches accesses the go repo at go.googlesource.com
	testenv.MustHaveExecPath(t, "git") // ResolveSupportedBranches uses the git command to do so.

	got, err := ResolveSupportedBranches()
	if err != nil {
		t.Fatal(err)
	}
	// We can't check the hashes because they change, but we can check the keys.
	for key := range got {
		if !SupportedBranches[key] {
			t.Errorf("got key %q not in SupportedBranches", key)
		}
	}
	if g, w := len(got), len(SupportedBranches); g != w {
		t.Errorf("got %d hashes, want %d", g, w)
	}
}
