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

var (
	// Regexp for matching go tags. The groups are:
	// 1  the major.minor version
	// 2  the patch version, or empty if none
	// 3  the entire prerelease, if present
	// 4  the prerelease type ("beta" or "rc")
	// 5  the prerelease number
	tagRegexp = regexp.MustCompile(`^go(\d+\.\d+)(\.\d+|)((beta|rc)(\d+))?$`)
)

// VersionForTag returns the semantic version for the Go tag, or "" if
// tag doesn't correspond to a Go release or beta tag.
// Examples:
//   "go1.2" => "v1.2.0"
//   "go1.13beta1" => "v1.13.0-beta.1"
//   "go1.9rc2" => "v1.9.0-rc.2"
func VersionForTag(tag string) string {
	m := tagRegexp.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	version := "v" + m[1]
	if m[2] != "" {
		version += m[2]
	} else {
		version += ".0"
	}
	if m[3] != "" {
		version += "-" + m[4] + "." + m[5]
	}
	return version
}

// TagForVersion returns the Go standard library repository tag corresponding
// to semver. The Go tags differ from standard semantic versions in a few ways,
// such as beginning with "go" instead of "v".
func TagForVersion(version string) (_ string, err error) {
	defer derrors.Wrap(&err, "TagForVersion(%q)", version)

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

// MajorVersionForVersion returns the Go major version for version.
// E.g. "v1.13.3" => "go1".
func MajorVersionForVersion(version string) (_ string, err error) {
	defer derrors.Wrap(&err, "MajorTagForVersion(%q)", version)

	tag, err := TagForVersion(version)
	if err != nil {
		return "", err
	}
	i := strings.IndexRune(tag, '.')
	if i < 0 {
		return "", fmt.Errorf("no '.' in go tag %q", tag)
	}
	return tag[:i], nil
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

const (
	GoRepoURL       = "https://go.googlesource.com/go"
	GoSourceRepoURL = "https://github.com/golang/go"
)

// UseTestData determines whether to really clone the Go repo, or use
// stripped-down versions of the repo from the testdata directory.
var UseTestData = false

// TestCommitTime is the time used for all commits when UseTestData is true.
var TestCommitTime = time.Date(2019, 9, 4, 1, 2, 3, 0, time.UTC)

// getGoRepo returns a repo object for the Go repo at version.
func getGoRepo(version string) (_ *git.Repository, err error) {
	tag, err := TagForVersion(version)
	if err != nil {
		return nil, err
	}
	return git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           GoRepoURL,
		ReferenceName: plumbing.NewTagReferenceName(tag),
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
}

// getTestGoRepo gets a Go repo for testing.
func getTestGoRepo(version string) (_ *git.Repository, err error) {
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

// Versions returns all the versions of Go that are relevant to the discovery
// site. These are all release versions (tags of the forms "goN.N" and
// "goN.N.N", where N is a number) and beta or rc versions (tags of the forms
// "goN.NbetaN" and "goN.N.NbetaN", and similarly for "rc" replacing "beta").
func Versions() (_ []string, err error) {
	defer derrors.Wrap(&err, "Versions()")

	var refNames []plumbing.ReferenceName
	if UseTestData {
		refNames = testRefs
	} else {
		re := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
			URLs: []string{GoRepoURL},
		})
		refs, err := re.List(&git.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, r := range refs {
			refNames = append(refNames, r.Name())
		}
	}

	var versions []string
	for _, name := range refNames {
		if !name.IsTag() {
			continue
		}
		v := VersionForTag(name.Short())
		if v != "" {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// Directory returns the directory of the standard library relative to the repo root.
func Directory(version string) string {
	// For versions older than v1.4.0-beta.1, the stdlib is in src/pkg.
	if semver.Compare(version, "v1.4.0-beta.1") == -1 {
		return "src/pkg"
	}
	return "src"
}

// Zip creates a module zip representing the entire Go standard library at the
// given version and returns a reader to it. It also returns the time of the
// commit for that version. The zip file is in module form, with each path
// prefixed by ModuleName + "@" + version.
//
// Zip reads the standard library at the Go repository tag corresponding to to
// the given semantic version.
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
	// Add files from the stdlib directory.
	libdir := root
	for _, d := range strings.Split(Directory(version), "/") {
		libdir, err = subTree(repo, libdir, d)
		if err != nil {
			return nil, time.Time{}, err
		}
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

// References used for Versions during testing.
var testRefs = []plumbing.ReferenceName{
	"refs/changes/56/93156/13",
	"refs/tags/weekly.2011-04-13",
	"refs/tags/go1.8rc2",
	"refs/tags/go1.8",
	"refs/tags/release.r59",
	"refs/tags/go1.9rc1",
	"refs/tags/go1.12.1",
	"refs/tags/go1.12.9",
	"refs/tags/go1.6beta1",
	"refs/tags/go1.6",
	"refs/tags/go1.13beta1",
	"refs/tags/go1.12",
	"refs/tags/go1.2.1",
	"refs/tags/go1.4.3",
	"refs/tags/go1.6.3",
	"refs/tags/go1.4.2",
	"refs/tags/go1.11",
	"refs/tags/go1.13",
}
