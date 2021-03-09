// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/symbol"
	"golang.org/x/pkgsite/internal/version"
)

// VersionsDetails contains the hierarchy of version summary information used
// to populate the version tab. Version information is organized into separate
// lists, one for each (ModulePath, Major Version) pair.
type VersionsDetails struct {
	// ThisModule is the slice of VersionLists with the same module path as the
	// current package.
	ThisModule []*VersionList

	// IncompatibleModules is the slice of the VersionsLists with the same
	// module path as the current package, but with incompatible versions.
	IncompatibleModules []*VersionList

	// OtherModules is the slice of VersionLists with a different module path
	// from the current package.
	OtherModules []string
}

// VersionListKey identifies a version list on the versions tab. We have a
// separate VersionList for each major version of a module series. Notably we
// have more version lists than module paths: v0 and v1 module versions are in
// separate version lists, despite having the same module path.
type VersionListKey struct {
	// ModulePath is the module path of this major version.
	ModulePath string

	// Major is the major version string (e.g. v1, v2)
	Major string

	// Incompatible indicates whether the VersionListKey represents an
	// incompatible module version.
	Incompatible bool

	// Deprecated indicates whether the major version is deprecated.
	Deprecated bool
	// DeprecationComment holds the reason for deprecation, if any.
	DeprecationComment string
}

// VersionList holds all versions corresponding to a unique (module path,
// major version) tuple in the version hierarchy.
type VersionList struct {
	VersionListKey
	// Versions holds the nested version summaries, organized in descending
	// semver order.
	Versions []*VersionSummary
}

// VersionSummary holds data required to format the version link on the
// versions tab.
type VersionSummary struct {
	CommitTime string
	// Link to this version, for use in the anchor href.
	Link                string
	Version             string
	Retracted           bool
	RetractionRationale string
	Symbols             []*Symbol
}

func fetchVersionsDetails(ctx context.Context, ds internal.DataSource, fullPath, modulePath string) (*VersionsDetails, error) {
	db, ok := ds.(*postgres.DB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return nil, proxydatasourceNotSupportedErr()
	}
	versions, err := db.GetVersionsForPath(ctx, fullPath)
	if err != nil {
		return nil, err
	}

	outVersionToNameToUnitSymbol := map[string]map[string]*internal.UnitSymbol{}
	if experiment.IsActive(ctx, internal.ExperimentSymbolHistoryVersionsPage) {
		versionToNameToUnitSymbols, err := db.GetPackageSymbols(ctx, fullPath, modulePath)
		if err != nil {
			return nil, err
		}
		outVersionToNameToUnitSymbol = symbol.IntroducedHistory(versionToNameToUnitSymbols)
	}
	linkify := func(mi *internal.ModuleInfo) string {
		// Here we have only version information, but need to construct the full
		// import path of the package corresponding to this version.
		var versionPath string
		if mi.ModulePath == stdlib.ModulePath {
			versionPath = fullPath
		} else {
			versionPath = pathInVersion(internal.V1Path(fullPath, modulePath), mi)
		}
		return constructUnitURL(versionPath, mi.ModulePath, linkVersion(mi.Version, mi.ModulePath))
	}
	return buildVersionDetails(ctx, modulePath, versions, outVersionToNameToUnitSymbol, linkify), nil
}

// pathInVersion constructs the full import path of the package corresponding
// to mi, given its v1 path. To do this, we first compute the suffix of the
// package path in the given module series, and then append it to the real
// (versioned) module path.
//
// For example: if we're considering package foo.com/v3/bar/baz, and encounter
// module version foo.com/bar/v2, we do the following:
//   1) Start with the v1Path foo.com/bar/baz.
//   2) Trim off the version series path foo.com/bar to get 'baz'.
//   3) Join with the versioned module path foo.com/bar/v2 to get
//      foo.com/bar/v2/baz.
// ...being careful about slashes along the way.
func pathInVersion(v1Path string, mi *internal.ModuleInfo) string {
	suffix := internal.Suffix(v1Path, mi.SeriesPath())
	if suffix == "" {
		return mi.ModulePath
	}
	return path.Join(mi.ModulePath, suffix)
}

// buildVersionDetails constructs the version hierarchy to be rendered on the
// versions tab, organizing major versions into those that have the same module
// path as the package version under consideration, and those that don't.  The
// given versions MUST be sorted first by module path and then by semver.
func buildVersionDetails(ctx context.Context,
	currentModulePath string,
	modInfos []*internal.ModuleInfo,
	versionToNameToSymbol map[string]map[string]*internal.UnitSymbol,
	linkify func(v *internal.ModuleInfo) string) *VersionsDetails {
	// lists organizes versions by VersionListKey. Note that major version isn't
	// sufficient as a key: there are packages contained in the same major
	// version of different modules, for example github.com/hashicorp/vault/api,
	// which exists in v1 of both of github.com/hashicorp/vault and
	// github.com/hashicorp/vault/api.
	lists := make(map[VersionListKey][]*VersionSummary)
	// seenLists tracks the order in which we encounter entries of each version
	// list. We want to preserve this order.
	var seenLists []VersionListKey
	for _, mi := range modInfos {
		// Try to resolve the most appropriate major version for this version. If
		// we detect a +incompatible version (when the path version does not match
		// the sematic version), we prefer the path version.
		major := semver.Major(mi.Version)
		if mi.ModulePath == stdlib.ModulePath {
			var err error
			major, err = stdlib.MajorVersionForVersion(mi.Version)
			if err != nil {
				panic(err)
			}
		}
		if _, pathMajor, ok := module.SplitPathVersion(mi.ModulePath); ok {
			// We prefer the path major version except for v1 import paths where the
			// semver major version is v0. In this case, we prefer the more specific
			// semver version.
			if pathMajor != "" {
				// Trim both '/' and '.' from the path major version to account for
				// standard and gopkg.in module paths.
				major = strings.TrimLeft(pathMajor, "/.")
			} else if version.IsIncompatible(mi.Version) {
				major = semver.Major(mi.Version)
			} else if major != "v0" && !strings.HasPrefix(major, "go") {
				major = "v1"
			}
		}
		key := VersionListKey{
			ModulePath:   mi.ModulePath,
			Major:        major,
			Incompatible: version.IsIncompatible(mi.Version),
		}
		vs := &VersionSummary{
			Link:       linkify(mi),
			CommitTime: absoluteTime(mi.CommitTime),
			Version:    linkVersion(mi.Version, mi.ModulePath),
		}
		if experiment.IsActive(ctx, internal.ExperimentRetractions) {
			key.Deprecated = mi.Deprecated
			key.DeprecationComment = mi.DeprecationComment
			vs.Retracted = mi.Retracted
			vs.RetractionRationale = mi.RetractionRationale
		}
		if nts, ok := versionToNameToSymbol[mi.Version]; ok {
			vs.Symbols = symbolsForVersion(linkify(mi), nts)
		}
		if _, ok := lists[key]; !ok {
			seenLists = append(seenLists, key)
		}
		lists[key] = append(lists[key], vs)
	}

	var details VersionsDetails
	other := map[string]bool{}
	for _, key := range seenLists {
		vl := &VersionList{
			VersionListKey: key,
			Versions:       lists[key],
		}
		if key.ModulePath == currentModulePath {
			if key.Incompatible {
				details.IncompatibleModules = append(details.IncompatibleModules, vl)
			} else {
				details.ThisModule = append(details.ThisModule, vl)
			}
		} else {
			other[key.ModulePath] = true
		}
	}
	for m := range other {
		details.OtherModules = append(details.OtherModules, m)
	}
	// Sort for testing.
	sort.Strings(details.OtherModules)
	return &details
}

// formatVersion formats a more readable representation of the given version
// string. On any parsing error, it simply returns the input unmodified.
//
// For pseudo versions, the version string will use a shorten commit hash of 7
// characters to identify the version, and hide timestamp using ellipses.
//
// For any version string longer than 25 characters, the pre-release string will be
// truncated, such that the string displayed is exactly 25 characters, including the ellipses.
//
// See TestFormatVersion for examples.
func formatVersion(v string) string {
	const maxLen = 25
	if len(v) <= maxLen {
		return v
	}
	vType, err := version.ParseType(v)
	if err != nil {
		log.Errorf(context.TODO(), "formatVersion(%q): error parsing version: %v", v, err)
		return v
	}
	if vType != version.TypePseudo {
		// If the version is release or prerelease, return a version string of
		// maxLen by truncating the end of the string. maxLen is inclusive of
		// the "..." characters.
		return v[:maxLen-3] + "..."
	}

	// The version string will have a max length of 25:
	// base: "vX.Y.Z-prerelease.0" = up to 15
	// ellipse: "..." = 3
	// commit: "-abcdefa" = 7
	commit := shorten(pseudoVersionRev(v), 7)
	base := shorten(pseudoVersionBase(v), 15)
	return fmt.Sprintf("%s...-%s", base, commit)
}

// shorten shortens the string s to maxLen by removing the trailing characters.
func shorten(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// pseudoVersionRev extracts the pseudo version base, excluding the timestamp.
// It assumes the pseudo version is correctly formatted.
//
// See TestPseudoVersionBase for examples.
func pseudoVersionBase(v string) string {
	parts := strings.Split(v, "-")
	if len(parts) != 3 {
		mid := strings.Join(parts[1:len(parts)-1], "-")
		parts = []string{parts[0], mid, parts[2]}
	}
	// The version string will always be split into one
	// of these 3 parts:
	// 1. [vX.0.0, yyyymmddhhmmss, abcdefabcdef]
	// 2. [vX.Y.Z, pre.0.yyyymmddhhmmss, abcdefabcdef]
	// 3. [vX.Y.Z, 0.yyyymmddhhmmss, abcdefabcdef]
	p := strings.Split(parts[1], ".")
	var suffix string
	if len(p) > 0 {
		// There is a "pre.0" or "0" prefix in the second element.
		suffix = strings.Join(p[0:len(p)-1], ".")
	}
	return fmt.Sprintf("%s-%s", parts[0], suffix)
}

// pseudoVersionRev extracts the first 7 characters of the commit identifier
// from a pseudo version string. It assumes the pseudo version is correctly
// formatted.
func pseudoVersionRev(v string) string {
	v = strings.TrimSuffix(v, "+incompatible")
	j := strings.LastIndex(v, "-")
	return v[j+1:]
}

// displayVersion returns the version string, formatted for display.
func displayVersion(v string, modulePath string) string {
	if modulePath == stdlib.ModulePath {
		if strings.HasPrefix(v, "v0.0.0") {
			return strings.Split(v, "-")[2]
		}
		return goTagForVersion(v)
	}
	return formatVersion(v)
}

// linkVersion returns the version string, suitable for use in
// a link to this site.
// TODO(golang/go#41855): Clarify definition / use case for linkVersion and
// other version strings.
func linkVersion(v string, modulePath string) string {
	if modulePath == stdlib.ModulePath {
		if strings.HasPrefix(v, "go") {
			return v
		}
		return goTagForVersion(v)
	}
	return v
}

// goTagForVersion returns the Go tag corresponding to a given semantic
// version. It should only be used if we are 100% sure the version will
// correspond to a Go tag, such as when we are fetching the version from the
// database.
func goTagForVersion(v string) string {
	tag, err := stdlib.TagForVersion(v)
	if err != nil {
		log.Errorf(context.TODO(), "goTagForVersion(%q): %v", v, err)
		return "unknown"
	}
	return tag
}
