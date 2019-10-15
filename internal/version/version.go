// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package version handles version types.
package version

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/discovery/internal/thirdparty/semver"
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

func (t Type) String() string {
	return string(t)
}

var pseudoVersionRE = regexp.MustCompile(`^v[0-9]+\.(0\.0-|\d+\.\d+-([^+]*\.)?0\.)\d{14}-[A-Za-z0-9]+(\+incompatible)?$`)

// IsPseudo reports whether a valid version v is a pseudo-version.
// Modified from src/cmd/go/internal/modfetch.
func IsPseudo(v string) bool {
	return strings.Count(v, "-") >= 2 && pseudoVersionRE.MatchString(v)
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
