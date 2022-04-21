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
	"unicode"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
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
// separate VersionList for each major version of a module series.
// Notably we have more version lists than module paths: v0 and v1 module
// versions are in separate version lists, despite having the same module path.
// Also note that major version isn't sufficient as a key: there are packages
// contained in the same major version of different modules, for example
// github.com/hashicorp/vault/api, which exists in v1 of both of
// github.com/hashicorp/vault and github.com/hashicorp/vault/api.
type VersionListKey struct {
	// ModulePath is the module path of this major version.
	ModulePath string

	// Major is the major version string (e.g. v1, v2)
	Major string

	// Incompatible indicates whether the VersionListKey represents an
	// incompatible module version.
	Incompatible bool
}

// VersionList holds all versions corresponding to a unique (module path,
// major version) tuple in the version hierarchy.
type VersionList struct {
	VersionListKey
	// Deprecated indicates whether the major version is deprecated.
	Deprecated bool
	// DeprecationComment holds the reason for deprecation, if any.
	DeprecationComment string
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
	IsMinor             bool
	Symbols             [][]*Symbol
	Vulns               []Vuln
}

func fetchVersionsDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, getVulnEntries vulnEntriesFunc) (*VersionsDetails, error) {
	db, ok := ds.(*postgres.DB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return nil, datasourceNotSupportedErr()
	}
	versions, err := db.GetVersionsForPath(ctx, um.Path)
	if err != nil {
		return nil, err
	}

	sh := internal.NewSymbolHistory()
	if !um.IsCommand() {
		sh, err = db.GetSymbolHistory(ctx, um.Path, um.ModulePath)
		if err != nil {
			return nil, err
		}
	}
	linkify := func(mi *internal.ModuleInfo) string {
		// Here we have only version information, but need to construct the full
		// import path of the package corresponding to this version.
		var versionPath string
		if mi.ModulePath == stdlib.ModulePath {
			versionPath = um.Path
		} else {
			versionPath = pathInVersion(internal.V1Path(um.Path, um.ModulePath), mi)
		}
		return constructUnitURL(versionPath, mi.ModulePath, linkVersion(mi.ModulePath, mi.Version, mi.Version))
	}
	return buildVersionDetails(ctx, um.ModulePath, versions, sh, linkify, getVulnEntries), nil
}

// pathInVersion constructs the full import path of the package corresponding
// to mi, given its v1 path. To do this, we first compute the suffix of the
// package path in the given module series, and then append it to the real
// (versioned) module path.
//
// For example: if we're considering package foo.com/v3/bar/baz, and encounter
// module version foo.com/bar/v2, we do the following:
//  1. Start with the v1Path foo.com/bar/baz.
//  2. Trim off the version series path foo.com/bar to get 'baz'.
//  3. Join with the versioned module path foo.com/bar/v2 to get
//     foo.com/bar/v2/baz.
//
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
func buildVersionDetails(ctx context.Context, currentModulePath string,
	modInfos []*internal.ModuleInfo,
	sh *internal.SymbolHistory,
	linkify func(v *internal.ModuleInfo) string,
	getVulnEntries vulnEntriesFunc,
) *VersionsDetails {
	// lists organizes versions by VersionListKey.
	lists := make(map[VersionListKey]*VersionList)
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
		// We prefer the path major version except for v1 import paths where the
		// semver major version is v0. In this case, we prefer the more specific
		// semver version.
		pathMajor := internal.MajorVersionForModule(mi.ModulePath)
		if pathMajor != "" {
			major = pathMajor
		} else if version.IsIncompatible(mi.Version) {
			major = semver.Major(mi.Version)
		} else if major != "v0" && !strings.HasPrefix(major, "go") {
			major = "v1"
		}
		key := VersionListKey{
			ModulePath:   mi.ModulePath,
			Major:        major,
			Incompatible: version.IsIncompatible(mi.Version),
		}
		commitTime := "date unknown"
		if !mi.CommitTime.IsZero() {
			commitTime = absoluteTime(mi.CommitTime)
		}
		vs := &VersionSummary{
			Link:                linkify(mi),
			CommitTime:          commitTime,
			Version:             linkVersion(mi.ModulePath, mi.Version, mi.Version),
			IsMinor:             isMinor(mi.Version),
			Retracted:           mi.Retracted,
			RetractionRationale: shortRationale(mi.RetractionRationale),
		}
		if sv := sh.SymbolsAtVersion(mi.Version); sv != nil {
			vs.Symbols = symbolsForVersion(linkify(mi), sv)
		}
		vs.Vulns = VulnsForPackage(mi.ModulePath, mi.Version, "", getVulnEntries)
		vl := lists[key]
		if vl == nil {
			seenLists = append(seenLists, key)
			vl = &VersionList{
				VersionListKey:     key,
				Deprecated:         mi.Deprecated,
				DeprecationComment: shortRationale(mi.DeprecationComment),
			}
			lists[key] = vl
		}
		vl.Versions = append(vl.Versions, vs)
	}

	var details VersionsDetails
	other := map[string]bool{}
	for _, key := range seenLists {
		vl := lists[key]
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

// isMinor reports whether v is a release version where the patch version is 0.
// It is assumed that v is a valid semantic version.
func isMinor(v string) bool {
	if version.IsIncompatible(v) {
		return false
	}
	typ, err := version.ParseType(v)
	if err != nil {
		// This should never happen because v will always be a valid semantic
		// version.
		return false
	}
	if typ == version.TypePrerelease || typ == version.TypePseudo {
		return false
	}
	return strings.HasSuffix(strings.TrimPrefix(v, semver.MajorMinor(v)), ".0")
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

// shortRationale returns a rationale string that is safe
// to print in a terminal. It returns hard-coded strings if the rationale
// is empty, too long, or contains non-printable characters.
func shortRationale(rationale string) string {
	// Copied with slight modifications from
	// https://go.googlesource.com/go/+/87c6fa4f473f178f7d931ddadd10c76444f8dc7b/src/cmd/go/internal/modload/modfile.go#208.
	const maxRationaleBytes = 500
	if i := strings.Index(rationale, "\n"); i >= 0 {
		rationale = rationale[:i]
	}
	rationale = strings.TrimSpace(rationale)
	if rationale == "" {
		return ""
	}
	if len(rationale) > maxRationaleBytes {
		return "(rationale omitted: too long)"
	}
	for _, r := range rationale {
		if !unicode.IsGraphic(r) && !unicode.IsSpace(r) {
			return "(rationale omitted: contains non-printable characters)"
		}
	}
	// NOTE: the go.mod parser rejects invalid UTF-8, so we don't check that here.
	return rationale
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
func displayVersion(modulePath, requestedVersion, resolvedVersion string) string {
	if modulePath == stdlib.ModulePath {
		if stdlib.SupportedBranches[requestedVersion] || strings.HasPrefix(resolvedVersion, "v0.0.0") {
			commit := strings.Split(resolvedVersion, "-")[2]
			// If the resolvedVersion is a pseudoversion and the
			// requestedVersion is not dev.fuzz, display "master (<commit>)".
			// std doesn't have actual pseudoversions, so the only ones we
			// support are "master" and "dev.fuzz".
			v := version.Master
			if requestedVersion == stdlib.DevFuzz ||
				requestedVersion == stdlib.DevBoringCrypto {
				v = requestedVersion
			}
			return fmt.Sprintf("%s (%s)", v, commit[0:7])
		}
		return goTagForVersion(resolvedVersion)
	}
	return formatVersion(resolvedVersion)
}

// linkVersion returns the version string, suitable for use in
// a link to this site.
// See TestLinkVersion for examples.
func linkVersion(modulePath, requestedVersion, resolvedVersion string) string {
	if modulePath == stdlib.ModulePath {
		if strings.HasPrefix(resolvedVersion, "go") {
			return resolvedVersion
		}
		if stdlib.SupportedBranches[requestedVersion] {
			return requestedVersion
		}
		return goTagForVersion(resolvedVersion)
	}
	return resolvedVersion
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
