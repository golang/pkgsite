// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdlib

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/discovery/internal/thirdparty/semver"

	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

func TestTagForVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
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
		t.Run(tc.name, func(t *testing.T) {
			got, err := TagForVersion(tc.version)
			if (err != nil) != tc.wantErr {
				t.Errorf("TagForVersion(%q) = %q, %v, wantErr %v", tc.version, got, err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("TagForVersion(%q) = %q, %v, wanted %q, %v", tc.version, got, err, tc.want, nil)
			}
		})
	}
}

func TestZipGoRepo(t *testing.T) {
	for _, version := range []string{"v1.12.0", "v1.3.2"} {
		t.Run(version, func(t *testing.T) {
			fs := osfs.New(filepath.Join("testdata", version))
			repo, err := git.Init(memory.NewStorage(), fs)
			if err != nil {
				t.Fatal(err)
			}
			wt, err := repo.Worktree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := wt.Add(""); err != nil {
				t.Fatal(err)
			}
			hash, err := wt.Commit("", &git.CommitOptions{All: true, Author: &object.Signature{
				Name:  "Joe Random",
				Email: "joe@example.com",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.CreateTag("go"+version[1:], hash, nil); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			if err := zipGoRepo(&buf, repo, version); err != nil {
				t.Fatal(err)
			}
			r := bytes.NewReader(buf.Bytes())
			zr, err := zip.NewReader(r, int64(r.Len()))
			if err != nil {
				t.Fatal(err)
			}
			wantFiles := map[string]bool{
				"LICENSE":               true,
				"errors/errors.go":      true,
				"errors/errors_test.go": true,
			}
			if semver.Compare(version, "v1.4.0") > 0 {
				wantFiles["README.md"] = true
			} else {
				wantFiles["README"] = true
			}

			wantPrefix := "std@" + version + "/"
			for _, f := range zr.File {
				if !strings.HasPrefix(f.Name, wantPrefix) {
					t.Errorf("filename %q missing prefix %q", f.Name, wantPrefix)
					continue
				}
				delete(wantFiles, f.Name[len(wantPrefix):])
			}
			if len(wantFiles) > 0 {
				t.Errorf("zip missing files: %v", reflect.ValueOf(wantFiles).MapKeys())
			}
		})
	}
}

func TestReleaseVersionForTag(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", ""},
		{"go1.9beta2", ""},
		{"go1.12", "v1.12.0"},
		{"go1.9.7", "v1.9.7"},
		{"go2.0", "v2.0.0"},
	} {
		got := releaseVersionForTag(tc.in)
		if got != tc.want {
			t.Errorf("releaseVersionForTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
