// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stdlib supports special handling of the Go standard library.
// Regardless of the how the standard library has been split into modules for
// development and testing, the discovery site treats it as a single module
// named "std".
package stdlib

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/testhelper"
	"golang.org/x/discovery/internal/thirdparty/semver"

	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/filemode"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

// ModulePath is the name of the module for the standard library.
const ModulePath = "std"

// TagForVersion returns the Go standard library repository tag corresponding
// to semver. The Go tags differ from standard semantic versions in a few ways,
// such as beginning with "go" instead of "v".
func TagForVersion(version string) (string, error) {
	if !semver.IsValid(version) {
		return "", derrors.FromHTTPStatus(http.StatusBadRequest, "requested version is not a valid semantic version: %q ", version)
	}
	goVersion := semver.Canonical(version)
	prerelease := semver.Prerelease(goVersion)
	versionWithoutPrerelease := strings.TrimSuffix(goVersion, prerelease)
	patch := strings.TrimPrefix(versionWithoutPrerelease, semver.MajorMinor(goVersion)+".")
	if patch == "0" {
		versionWithoutPrerelease = strings.TrimSuffix(versionWithoutPrerelease, ".0")
	}

	goVersion = fmt.Sprintf("go%s", strings.TrimPrefix(versionWithoutPrerelease, "v"))
	if prerelease != "" {
		// Go prereleases look like  "beta1" instead of "beta.1".
		// "beta1" is bad for sorting (since beta10 comes before beta9), so
		// require the dot form.
		i := finalDigitsIndex(prerelease)
		if i >= 1 {
			if prerelease[i-1] != '.' {
				return "", derrors.FromHTTPStatus(http.StatusBadRequest, "final digits in a prerelease must follow a period")
			}
			// Remove the dot.
			prerelease = prerelease[:i-1] + prerelease[i:]
		}
		goVersion += strings.TrimPrefix(prerelease, "-")
	}
	return goVersion, nil
}

// finalDigitsIndex returns the index of the first digit in the sequence of digits ending s.
// If s doesn't end in digits, it returns -1.
func finalDigitsIndex(s string) int {
	// Assume ASCII (since the semver package does anyway).
	var i int
	for i = len(s) - 1; i >= 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			break
		}
	}
	if i == len(s)-1 {
		return -1
	}
	return i + 1
}

const goRepoURL = "https://go.googlesource.com/go"

// UseTestData determines whether to really clone the Go repo, or use
// stripped-down versions of the repo from the testdata directory.
var UseTestData = false

// TestCommitTime is the time used for all commits when UseTestData is true.
var TestCommitTime = time.Date(2019, 9, 4, 1, 2, 3, 0, time.UTC)

// getGoRepo returns a repo object for the Go repo at version.
func getGoRepo(version string) (_ *git.Repository, err error) {
	var tag string
	if version == "" {
		tag, err = latestGoReleaseTag()
	} else {
		tag, err = TagForVersion(version)
	}
	if err != nil {
		return nil, err
	}
	return git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           goRepoURL,
		ReferenceName: plumbing.NewTagReferenceName(tag),
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
}

// getTestGoRepo gets a Go repo for testing.
func getTestGoRepo(version string) (_ *git.Repository, err error) {
	if version == "" {
		return nil, errors.New("empty version not supported in tests")
	}
	fs := osfs.New(filepath.Join(testhelper.TestDataPath("testdata"), version))
	repo, err := git.Init(memory.NewStorage(), fs)
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	// Add all files in the directory.
	if _, err := wt.Add(""); err != nil {
		return nil, err
	}
	_, err = wt.Commit("", &git.CommitOptions{All: true, Author: &object.Signature{
		Name:  "Joe Random",
		Email: "joe@example.com",
		When:  TestCommitTime,
	}})
	if err != nil {
		return nil, err
	}
	return repo, nil
}

// latestGoReleaseTag returns the tag of the latest Go release.
// Only tags of the forms "goN.N" and "goN.N.N", where N is a number, are
// considered. Prerelease and other tags are ignored.
func latestGoReleaseTag() (string, error) {
	re := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{goRepoURL},
	})
	refs, err := re.List(&git.ListOptions{})
	if err != nil {
		return "", err
	}
	latestSemver := "v0.0.0"
	latestTag := ""
	for _, ref := range refs {
		name := ref.Name()
		if !name.IsTag() {
			continue
		}
		tag := name.Short()
		v := releaseVersionForTag(tag)
		if v == "" {
			continue
		}
		if semver.Compare(latestSemver, v) < 0 {
			latestSemver = v
			latestTag = tag
		}
	}
	return latestTag, nil
}

var (
	minorRegexp = regexp.MustCompile(`^go(\d+\.\d+)$`)
	patchRegexp = regexp.MustCompile(`^go(\d+\.\d+\.\d+)$`)
)

// releaseVersionForTag returns the semantic version for the Go tag, or "" if
// tag doesn't correspond to a Go release tag.
// Examples:
//   "go1.2" => "v1.2.0"
//   "go1.9beta2" => ""
func releaseVersionForTag(tag string) string {
	if m := minorRegexp.FindStringSubmatch(tag); m != nil {
		return "v" + m[1] + ".0"
	}
	if m := patchRegexp.FindStringSubmatch(tag); m != nil {
		return "v" + m[1]
	}
	return ""
}

// Zip creates a module zip representing the entire Go standard library at the
// given version and returns a reader to it. It also returns the time of the
// commit for that version. The zip file is in module form, with each path
// prefixed by ModuleName + "@" + version.
//
// Zip reads the standard library at the Go repository tag corresponding to to
// the given semantic version. If version is empty, it uses the latest released
// version.
//
// Zip ignores go.mod files in the standard library, treating it as if it were a
// single module named "std" at the given version.
func Zip(version string) (_ *zip.Reader, commitTime time.Time, err error) {
	// This code taken, with modifications, from
	// https://github.com/shurcooL/play/blob/master/256/moduleproxy/std/std.go.
	defer derrors.Wrap(&err, "stdlib.Zip(%q)", version)

	var repo *git.Repository
	if UseTestData {
		repo, err = getTestGoRepo(version)
	} else {
		repo, err = getGoRepo(version)
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	head, err := repo.Head()
	if err != nil {
		return nil, time.Time{}, err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, time.Time{}, err
	}
	root, err := repo.TreeObject(commit.TreeHash)
	if err != nil {
		return nil, time.Time{}, err
	}
	prefixPath := ModulePath + "@" + version
	// Add top-level files.
	if err := addFiles(z, repo, root, prefixPath, false); err != nil {
		return nil, time.Time{}, err
	}
	// If there is src/pkg, add that; otherwise, add src.
	var libdir *object.Tree
	src, err := subTree(repo, root, "src")
	if err != nil {
		return nil, time.Time{}, err
	}
	pkg, err := subTree(repo, src, "pkg")
	if err == os.ErrNotExist {
		libdir = src
	} else if err != nil {
		return nil, time.Time{}, err
	} else {
		libdir = pkg
	}
	if err := addFiles(z, repo, libdir, prefixPath, true); err != nil {
		return nil, time.Time{}, err
	}

	if err := z.Close(); err != nil {
		return nil, time.Time{}, err
	}
	br := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(br, int64(br.Len()))
	if err != nil {
		return nil, time.Time{}, err
	}
	return zr, commit.Committer.When, nil
}

// addFiles adds the files in t to z, using dirpath as the path prefix.
// If recursive is true, it also adds the files in all subdirectories.
func addFiles(z *zip.Writer, r *git.Repository, t *object.Tree, dirpath string, recursive bool) error {
	for _, e := range t.Entries {
		if strings.HasPrefix(e.Name, ".") || strings.HasPrefix(e.Name, "_") {
			continue
		}
		switch e.Mode {
		case filemode.Regular, filemode.Executable:
			blob, err := r.BlobObject(e.Hash)
			if err != nil {
				return err
			}
			dst, err := z.Create(path.Join(dirpath, e.Name))
			if err != nil {
				return err
			}
			src, err := blob.Reader()
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, src); err != nil {
				_ = src.Close()
				return err
			}
			if err := src.Close(); err != nil {
				return err
			}
		case filemode.Dir:
			if !recursive || e.Name == "testdata" {
				continue
			}
			t2, err := r.TreeObject(e.Hash)
			if err != nil {
				return err
			}
			if err := addFiles(z, r, t2, path.Join(dirpath, e.Name), recursive); err != nil {
				return err
			}
		}
	}
	return nil
}

// subTree looks non-recursively for a directory with the given name in t,
// and returns the corresponding tree.
// If a directory with such name doesn't exist in t, it returns os.ErrNotExist.
func subTree(r *git.Repository, t *object.Tree, name string) (*object.Tree, error) {
	for _, e := range t.Entries {
		if e.Name == name {
			return r.TreeObject(e.Hash)
		}
	}
	return nil, os.ErrNotExist
}
