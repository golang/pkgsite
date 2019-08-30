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
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/thirdparty/semver"

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

// Zip writes a module zip representing the entire Go standard library to w.
// It reads the standard library at the Go repository tag corresponding to
// to the given semantic version. If version is empty, it uses the
// latest released version.
//
// Zip ignores go.mod files in the standard library, treating it as if it were a
// single module named "std" at the given version.
func Zip(w io.Writer, version string) (err error) {
	// This code taken, with modifications, from
	// https://github.com/shurcooL/play/blob/master/256/moduleproxy/std/std.go.
	defer derrors.Wrap(&err, "stdlib.Zip(w, %q)", version)

	var tag string
	if version == "" {
		tag, err = latestGoReleaseTag()
	} else {
		tag, err = TagForVersion(version)
	}
	if err != nil {
		return err
	}
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           goRepoURL,
		ReferenceName: plumbing.NewTagReferenceName(tag),
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
	if err != nil {
		return err
	}
	return zipGoRepo(w, repo, version)
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

// zipGoRepo writes a zip file of the Go standard library in r to w.
// The zip file is in module form, with each path prefixed by ModuleName + "@" + version.
//
// zipGoRepo assumes that the repo follows the layout of the Go repo, with all
// the Go files of the standard library in a subdirectory, either /src or /src/pkg.
func zipGoRepo(w io.Writer, r *git.Repository, version string) error {
	z := zip.NewWriter(w)
	head, err := r.Head()
	if err != nil {
		return err
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		return err
	}
	root, err := r.TreeObject(commit.TreeHash)
	if err != nil {
		return err
	}
	prefixPath := ModulePath + "@" + version
	// Add top-level files.
	if err := addFiles(z, r, root, prefixPath, false); err != nil {
		return err
	}
	// If there is src/pkg, add that; otherwise, add src.
	var libdir *object.Tree
	src, err := subTree(r, root, "src")
	if err != nil {
		return err
	}
	pkg, err := subTree(r, src, "pkg")
	if err == os.ErrNotExist {
		libdir = src
	} else if err != nil {
		return err
	} else {
		libdir = pkg
	}
	if err := addFiles(z, r, libdir, prefixPath, true); err != nil {
		return err
	}
	return z.Close()
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
