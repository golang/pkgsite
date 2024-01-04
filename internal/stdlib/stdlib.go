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
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

const (
	// ModulePath is the name of the module for the standard library.
	ModulePath = "std"

	// DevFuzz is the branch name for fuzzing in beta.
	DevFuzz = "dev.fuzz"

	// DevBoringCrypto is the branch name for dev.boringcrypto.
	DevBoringCrypto = "dev.boringcrypto"
)

var (
	// Regexp for matching go tags. The groups are:
	// 1  the major.minor version
	// 2  the patch version, or empty if none
	// 3  the entire prerelease, if present
	// 4  the prerelease type ("beta" or "rc")
	// 5  the prerelease number
	tagRegexp = regexp.MustCompile(`^go(\d+\.\d+)(\.\d+|)((beta|rc)(\d+))?$`)
)

// SupportedBranches are the branches of the stdlib repo supported by pkgsite.
var SupportedBranches = map[string]bool{
	version.Master:  true,
	DevBoringCrypto: true,
	DevFuzz:         true,
}

// VersionForTag returns the semantic version for the Go tag, or "" if
// tag doesn't correspond to a Go release or beta tag. In special cases,
// when the tag specified is either `latest` or `master` it will return the tag.
// Examples:
//
//	"go1" => "v1.0.0"
//	"go1.2" => "v1.2.0"
//	"go1.13beta1" => "v1.13.0-beta.1"
//	"go1.9rc2" => "v1.9.0-rc.2"
//	"latest" => "latest"
//	"master" => "master"
func VersionForTag(tag string) string {
	// Special cases for go1.
	if tag == "go1" {
		return "v1.0.0"
	}
	if tag == "go1.0" {
		return ""
	}
	// Special case for latest and master.
	if tag == version.Latest || SupportedBranches[tag] {
		return tag
	}
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
//
// Starting with go1.21.0, the first patch release of major go versions include
// the .0 suffix. Previously, the .0 suffix was elided (golang/go#57631).
func TagForVersion(v string) (_ string, err error) {
	defer derrors.Wrap(&err, "TagForVersion(%q)", v)

	// Special case: master => master or dev.fuzz => dev.fuzz
	if SupportedBranches[v] {
		return v, nil
	}
	if strings.HasPrefix(v, "v0.0.0") {
		return version.Master, nil
	}
	// Special case: v1.0.0 => go1.
	if v == "v1.0.0" {
		return "go1", nil
	}
	if !semver.IsValid(v) {
		return "", fmt.Errorf("%w: requested version is not a valid semantic version: %q ", derrors.InvalidArgument, v)
	}
	goVersion := semver.Canonical(v)
	prerelease := semver.Prerelease(goVersion)
	versionWithoutPrerelease := strings.TrimSuffix(goVersion, prerelease)
	patch := strings.TrimPrefix(versionWithoutPrerelease, semver.MajorMinor(goVersion)+".")
	if patch == "0" && (semver.Compare(v, "v1.21.0") < 0 || prerelease != "") {
		// Starting with go1.21.0, the first patch version includes .0.
		// Prereleases do not include .0 (we don't do prereleases for other patch releases).
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
				return "", fmt.Errorf("%w: final digits in a prerelease must follow a period", derrors.InvalidArgument)
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
	defer derrors.Wrap(&err, "MajorVersionForVersion(%q)", version)

	tag, err := TagForVersion(version)
	if err != nil {
		return "", err
	}
	if tag == "go1" || tag == "master" {
		return "go1", nil
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
	GoSourceRepoURL = "https://cs.opensource.google/go/go"

	GitHubRepo = "github.com/golang/go"
)

// TestCommitTime is the time used for all commits when UseTestData is true.
var (
	TestCommitTime     = time.Date(2019, 9, 4, 1, 2, 3, 0, time.UTC)
	TestMasterVersion  = "v0.0.0-20190904010203-89fb59e2e920"
	TestDevFuzzVersion = "v0.0.0-20190904010203-12de34vf56uz"
)

var (
	goRepoMu  sync.Mutex
	theGoRepo goRepo = &remoteGoRepo{}
)

func getGoRepo() goRepo {
	goRepoMu.Lock()
	defer goRepoMu.Unlock()
	return theGoRepo
}

func swapGoRepo(gr goRepo) goRepo {
	goRepoMu.Lock()
	defer goRepoMu.Unlock()
	old := theGoRepo
	theGoRepo = gr
	return old
}

// WithTestData arranges for this package to use a testing version of the Go repo.
// The returned function restores the previous state. Use with defer:
//
//	defer WithTestData()()
func WithTestData() func() {
	return withGoRepo(&testGoRepo{})
}

func withGoRepo(gr goRepo) func() {
	old := swapGoRepo(gr)
	return func() {
		swapGoRepo(old)
	}
}

// SetGoRepoPath tells this package to obtain the Go repo from the
// local filesystem at path, instead of cloning it.
func SetGoRepoPath(path string) error {
	gr := newLocalGoRepo(path)
	swapGoRepo(gr)
	return nil
}

func refNameForVersion(v string) (string, error) {
	if v == version.Master {
		return "HEAD", nil
	}
	if SupportedBranches[v] {
		return "refs/heads/" + v, nil
	}
	tag, err := TagForVersion(v)
	if err != nil {
		return "", err
	}
	return "refs/tags/" + tag, nil
}

// Versions returns all the semantic versions of Go that are relevant to the
// discovery site. These are all release versions (derived from tags of the
// forms "goN.N" and "goN.N.N", where N is a number) and beta or rc versions
// (derived from tags of the forms "goN.NbetaN" and "goN.N.NbetaN", and
// similarly for "rc" replacing "beta").
func Versions() (_ []string, err error) {
	defer derrors.Wrap(&err, "stdlib.Versions()")

	refs, err := getGoRepo().refs(context.TODO())
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, r := range refs {
		if !strings.HasPrefix(r.name, "refs/tags/") {
			continue
		}
		tagName := strings.TrimPrefix(r.name, "refs/tags/")
		v := VersionForTag(tagName)
		if v != "" {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// ResolveSupportedBranches returns the current hashes for each ref in
// SupportedBranches.
func ResolveSupportedBranches() (_ map[string]string, err error) {
	defer derrors.Wrap(&err, "ResolveSupportedBranches")

	refs, err := getGoRepo().refs(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("getting refs: %v", err)
	}
	m := map[string]string{}
	for _, r := range refs {
		if !strings.HasPrefix(r.name, "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(r.name, "refs/heads/")
		if SupportedBranches[name] {
			m[name] = r.hash
		}
	}
	return m, nil
}

// Directory returns the directory of the standard library relative to the repo root.
func Directory(v string) string {
	if semver.Compare(v, "v1.4.0-beta.1") >= 0 ||
		SupportedBranches[v] || strings.HasPrefix(v, "v0.0.0") {
		return "src"
	}
	// For versions older than v1.4.0-beta.1, the stdlib is in src/pkg.
	return "src/pkg"
}

// EstimatedZipSize is the approximate size of
// Zip("v1.15.2").
const EstimatedZipSize = 16 * 1024 * 1024

// ZipInfo returns the proxy .info information for the module std.
func ZipInfo(requestedVersion string) (resolvedVersion string, err error) {
	defer derrors.Wrap(&err, "stdlib.ZipInfo(%q)", requestedVersion)

	resolvedVersion, err = semanticVersion(requestedVersion)
	if err != nil {
		return "", err
	}
	return resolvedVersion, nil
}

func commiterTime(ctx context.Context, dir, object string) (time.Time, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--no-patch", "--no-notes", "--format=%aI", object)
	cmd.Dir = dir
	b, err := cmd.Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("running git show: %v, %s", err, b)
	}
	t, err := time.Parse(time.RFC3339, string(bytes.TrimSpace(b)))
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time output %q from command %v: %s", b, cmd, err)
	}
	return t, nil
}

func zipInternal(ctx context.Context, requestedVersion string) (_ *zip.Reader, resolvedVersion string, commitTime time.Time, prefix string, err error) {
	if requestedVersion == version.Latest {
		requestedVersion, err = semanticVersion(requestedVersion)
		if err != nil {
			return nil, "", time.Time{}, "", err
		}
	}
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, "", time.Time{}, "", err
	}
	defer func() {
		rmallerr := os.RemoveAll(dir)
		if err == nil {
			err = rmallerr
		}
	}()
	hash, err := getGoRepo().clone(ctx, requestedVersion, dir)
	if err != nil {
		return nil, "", time.Time{}, "", err
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)

	commitTime, err = commiterTime(ctx, dir, hash)
	if err != nil {
		return nil, "", time.Time{}, "", err
	}
	resolvedVersion = requestedVersion
	if SupportedBranches[requestedVersion] {
		resolvedVersion = newPseudoVersion("v0.0.0", commitTime, hash)
	}
	prefixPath := ModulePath + "@" + requestedVersion
	// Add top-level files.
	if err := addFiles(z, dir, prefixPath, false); err != nil {
		return nil, "", time.Time{}, "", err
	}
	// Add files from the stdlib directory.
	libDir := filepath.Join(dir, Directory(resolvedVersion))
	if err := addFiles(z, libDir, prefixPath, true); err != nil {
		return nil, "", time.Time{}, "", err
	}
	if err := z.Close(); err != nil {
		return nil, "", time.Time{}, "", err
	}
	br := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(br, int64(br.Len()))
	if err != nil {
		return nil, "", time.Time{}, "", err
	}
	return zr, resolvedVersion, commitTime, prefixPath, nil
}

// ContentDir creates an fs.FS representing the entire Go standard library at the
// given version (which must have been resolved with ZipInfo) and returns a
// reader to it. It also returns the time of the commit for that version.
//
// Normally, ContentDir returns the resolved version it was passed. If the
// resolved version is a supported branch like "master", ContentDir returns a
// semantic version for the branch.
//
// ContentDir reads the standard library at the Go repository tag corresponding
// to the given semantic version.
//
// ContentDir ignores go.mod files in the standard library, treating it as if it
// were a single module named "std" at the given version.
func ContentDir(ctx context.Context, requestedVersion string) (_ fs.FS, resolvedVersion string, commitTime time.Time, err error) {
	defer derrors.Wrap(&err, "stdlib.ContentDir(%q)", requestedVersion)

	zr, resolvedVersion, commitTime, prefix, err := zipInternal(ctx, requestedVersion)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	cdir, err := fs.Sub(zr, prefix)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return cdir, resolvedVersion, commitTime, nil
}

const pseudoHashLen = 12

func newPseudoVersion(version string, commitTime time.Time, hash string) string {
	return fmt.Sprintf("%s-%s-%s", version, commitTime.Format("20060102150405"), hash[:pseudoHashLen])
}

// VersionMatchesHash reports whether v is a pseudo-version whose hash
// part matches the prefix of the given hash.
func VersionMatchesHash(v, hash string) bool {
	if !version.IsPseudo(v) {
		return false
	}
	return v[len(v)-pseudoHashLen:] == hash[:pseudoHashLen]
}

// semanticVersion returns the semantic version corresponding to the
// requestedVersion. If the requested version is version.Master, then semanticVersion
// returns it as is. The branch name is resolved to a proper pseudo-version in
// Zip.
func semanticVersion(requestedVersion string) (_ string, err error) {
	defer derrors.Wrap(&err, "semanticVersion(%q)", requestedVersion)

	if SupportedBranches[requestedVersion] {
		return requestedVersion, nil
	}

	knownVersions, err := Versions()
	if err != nil {
		return "", err
	}

	switch requestedVersion {
	case version.Latest:
		var latestVersion string
		for _, v := range knownVersions {
			if !strings.HasPrefix(v, "v") {
				continue
			}
			versionType, err := version.ParseType(v)
			if err != nil {
				return "", err
			}
			if versionType != version.TypeRelease {
				// We expect there to always be at least 1 release version.
				continue
			}
			if semver.Compare(v, latestVersion) > 0 {
				latestVersion = v
			}
		}
		return latestVersion, nil
	default:
		for _, v := range knownVersions {
			if v == requestedVersion {
				return requestedVersion, nil
			}
		}
	}

	return "", fmt.Errorf("%w: requested version unknown: %q", derrors.InvalidArgument, requestedVersion)
}

// addFiles adds the files in t to z, using dirpath as the path prefix.
// If recursive is true, it also adds the files in all subdirectories.
func addFiles(z *zip.Writer, directory string, dirpath string, recursive bool) (err error) {
	defer derrors.Wrap(&err, "addFiles(zip, repository, tree, %q, %t)", dirpath, recursive)

	dirents, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, e := range dirents {
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if e.Name() == "go.mod" {
			// Ignore; we don't need it.
			continue
		}
		if strings.HasPrefix(e.Name(), "README") && !strings.Contains(dirpath, "/") {
			// For versions newer than v1.4.0-beta.1, the stdlib is in src/pkg.
			// This means that our construction of the zip files will return
			// two READMEs at the root:
			// https://golang.org/README.md and
			// https://golang.org/src/README.vendor
			//
			// We do not want to display the README.md
			// or any README.vendor.
			// However, we do want to store the README in
			// other directories.
			continue
		}
		switch {
		case e.Type().IsRegular():
			f, err := os.Open(filepath.Join(directory, e.Name()))
			if err != nil {
				return err
			}
			if err := writeZipFile(z, path.Join(dirpath, e.Name()), f); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case e.Type().IsDir():
			if !recursive || e.Name() == "testdata" {
				continue
			}
			if err != nil {
				return err
			}
			if err := addFiles(z, filepath.Join(directory, e.Name()), path.Join(dirpath, e.Name()), recursive); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeZipFile(z *zip.Writer, pathname string, src io.Reader) (err error) {
	defer derrors.Wrap(&err, "writeZipFile(zip, %q, src)", pathname)

	dst, err := z.Create(pathname)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

// Contains reports whether the given import path could be part of the Go standard library,
// by reporting whether the first component lacks a '.'.
func Contains(path string) bool {
	if i := strings.IndexByte(path, '/'); i != -1 {
		path = path[:i]
	}
	return !strings.Contains(path, ".")
}
