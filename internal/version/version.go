// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package version handles version types.
package version

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// Type defines the version types a module can have.
// This must be kept in sync with the 'version_type' database enum.
type Type string

const (
	// TypeRelease is a normal release.
	TypeRelease = Type("release")

	// TypePrerelease is a version with a prerelease.
	TypePrerelease = Type("prerelease")

	// TypePseudo appears to have a prerelease of the
	// form <commit date>-<commit hash>.
	TypePseudo = Type("pseudo")
)

const (
	// Latest signifies the latest available version in requests to the
	// proxy client.
	Latest = "latest"

	// Main represents the main branch.
	Main = "main"

	// Master represents the master branch.
	Master = "master"
)

func (t Type) String() string {
	return string(t)
}

var pseudoVersionRE = regexp.MustCompile(`^v[0-9]+\.(0\.0-|\d+\.\d+-([^+]*\.)?0\.)\d{14}-[A-Za-z0-9]+(\+incompatible)?$`)

// IsPseudo reports whether a valid version v is a pseudo-version.
// Modified from src/cmd/go/internal/modfetch.
func IsPseudo(v string) bool {
	return strings.Count(v, "-") >= 2 && pseudoVersionRE.MatchString(v)
}

// IsIncompatible reports whether a valid version v is an incompatible version.
func IsIncompatible(v string) bool {
	return strings.HasSuffix(v, "+incompatible")
}

// ParseType returns the Type of a given a version.
func ParseType(version string) (Type, error) {
	if !semver.IsValid(version) {
		return "", fmt.Errorf("ParseType(%q): invalid semver", version)
	}

	switch {
	case IsPseudo(version):
		return TypePseudo, nil
	case semver.Prerelease(version) != "":
		return TypePrerelease, nil
	default:
		return TypeRelease, nil
	}
}

// ForSorting returns a string that encodes version, so that comparing two such
// strings follows SemVer precedence, https://semver.org clause 11. It assumes
// version is valid. The returned string ends in '~' if and only if the version
// does not have a prerelease.
//
// For examples, see TestForSorting.
func ForSorting(version string) string {
	bytes := make([]byte, 0, len(version))
	prerelease := false // we are in the prerelease part
	nondigit := false   // this part has a non-digit character
	start := 1          // skip 'v'
	last := len(version)

	// Add the semver component version[start:end] to the result.
	addPart := func(end int) {
		if len(bytes) > 0 {
			// ',' comes before '-' and all letters and digits, so it correctly
			// imposes lexicographic ordering on the parts of the version.
			bytes = append(bytes, ',')
		}
		if nondigit {
			// Prepending the largest printable character '~' to a non-numeric
			// part, along with the fact that encoded numbers never begin with a
			// '~', (see appendNumericPrefix), ensures the semver requirement
			// that numeric identifiers always have lower precedence than
			// non-numeric ones.
			bytes = append(bytes, '~')
		} else {
			bytes = appendNumericPrefix(bytes, end-start)
		}
		bytes = append(bytes, version[start:end]...)
		start = end + 1 // skip over separator character
		nondigit = false
	}

loop:
	for i, c := range version[start:] {
		p := i + 1
		switch {
		case c == '.': // end of a part
			addPart(p)
		case c == '-': // first one is start of prerelease
			if !prerelease {
				prerelease = true
				addPart(p)
			} else {
				nondigit = true
			}
		case c == '+': // start of build; nothing after this matters
			last = p
			break loop

		case c < '0' || c > '9':
			nondigit = true
		}
	}
	if start < last {
		addPart(last)
	}
	if !prerelease {
		// Make sure prereleases appear first.
		bytes = append(bytes, '~')
	}
	return string(bytes)
}

// appendNumericPrefix appends a string representing n to dst.
// n is the length of a digit string; the value we append is a prefix for the
// digit string s such that
//
//	prefix1 + s1 < prefix2 + s2
//
// if and only if the integer denoted by s1 is less than the one denoted by s2.
// In other words, prefix + s is a string that can be compared with other such
// strings while preserving the ordering of the numbers.
//
// If n==1, there is no prefix. (Single-digit numbers are unchanged.)
// Otherwise, the prefix is a sequence of lower-case letters encoding n.
// Examples:
//
//	n    prefix
//	1    <none>
//	2    a
//	27   z
//	28   za
//	53   zz
//	54   zza
//
// This encoding depends on the ASCII properties that:
// - digits are ordered numerically
// - letters are ordered alphabetically
// - digits order before letters (so "1" < "a10")
func appendNumericPrefix(dst []byte, n int) []byte {
	n--
	for i := 0; i < n/26; i++ {
		dst = append(dst, 'z')
	}
	if rem := n % 26; rem > 0 {
		dst = append(dst, byte('a'+rem-1))
	}
	return dst
}

// Later reports whether v1 is later than v2, using semver but preferring
// release versions to pre-release versions, and both to pseudo-versions.
func Later(v1, v2 string) bool {
	rel1 := semver.Prerelease(v1) == ""
	rel2 := semver.Prerelease(v2) == ""
	if rel1 && rel2 {
		return semver.Compare(v1, v2) > 0
	}
	if rel1 != rel2 {
		return rel1
	}
	// Both are pre-release.
	pseudo1 := IsPseudo(v1)
	pseudo2 := IsPseudo(v2)
	if pseudo1 == pseudo2 {
		return semver.Compare(v1, v2) > 0
	}
	return !pseudo1
}

// LatestVersion finds the latest version of a module using the same algorithm as the
// Go command. It prefers tagged release versions to tagged pre-release
// versions, and both of those to pseudo-versions. If versions is empty, LatestVersion
// returns the empty string.
//
// hasGoMod should report whether the version it is given has a go.mod file.
// LatestVersion returns the latest incompatible version only if the latest compatible
// version does not have a go.mod file.
//
// The meaning of latest is defined at
// https://golang.org/ref/mod#version-queries. That definition does not deal
// with retractions, or with a subtlety involving incompatible versions. The
// actual definition is embodied in the go command's queryMatcher.filterVersions
// method. This function is a re-implementation and specialization of that
// method at Go version 1.16
// (https://go.googlesource.com/go/+/refs/tags/go1.16/src/cmd/go/internal/modload/query.go#441).
func LatestVersion(versions []string, hasGoMod func(v string) (bool, error)) (v string, err error) {
	latest := LatestOf(versions)
	if latest == "" {
		return "", nil
	}

	// If the latest is a compatible version, use it.
	if !IsIncompatible(latest) {
		return latest, nil
	}
	// The latest version is incompatible. If there is a go.mod file at the
	// latest compatible tagged version, assume the module author has adopted
	// proper versioning, and use that latest compatible version. Otherwise, use
	// this incompatible version.
	latestCompat := LatestOf(RemoveIf(versions, func(v string) bool { return IsIncompatible(v) || IsPseudo(v) }))
	if latestCompat == "" {
		// No compatible versions; use the latest (incompatible) version.
		return latest, nil
	}
	latestCompatHasGoMod, err := hasGoMod(latestCompat)
	if err != nil {
		return "", err
	}
	if latestCompatHasGoMod {
		return latestCompat, nil
	}
	return latest, nil
}

// LatestOf returns the latest version of a module from a list of versions, using
// the go command's definition of latest: semver is observed, except that
// release versions are preferred to prerelease, and both are preferred to pseudo-versions.
// If versions is empty, the empty string is returned.
func LatestOf(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if Later(v, latest) {
			latest = v
		}
	}
	return latest
}

// RemoveIf returns a copy of s that omits all values for which f returns true.
func RemoveIf(s []string, f func(string) bool) []string {
	var r []string
	for _, x := range s {
		if !f(x) {
			r = append(r, x)
		}
	}
	return r
}
