// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"path"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
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

	// OtherModules is the slice of VersionLists with a different module path
	// from the current package.
	OtherModules []*VersionList
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
	Link    string
	Version string
}

// legacyFetchModuleVersionsDetails builds a version hierarchy for module versions
// with the same series path as the given version.
func legacyFetchModuleVersionsDetails(ctx context.Context, ds internal.DataSource, mi *internal.ModuleInfo) (*VersionsDetails, error) {
	versions, err := ds.LegacyGetTaggedVersionsForModule(ctx, mi.ModulePath)
	if err != nil {
		return nil, err
	}
	// If no tagged versions of the module are found, fetch pseudo-versions
	// instead.
	if len(versions) == 0 {
		versions, err = ds.LegacyGetPsuedoVersionsForModule(ctx, mi.ModulePath)
		if err != nil {
			return nil, err
		}
	}
	linkify := func(m *internal.ModuleInfo) string {
		return constructModuleURL(m.ModulePath, linkVersion(m.Version, m.ModulePath))
	}
	return buildVersionDetails(mi.ModulePath, versions, linkify), nil
}

// legacyFetchPackageVersionsDetails builds a version hierarchy for all module
// versions containing a package path with v1 import path matching the given v1 path.
func legacyFetchPackageVersionsDetails(ctx context.Context, ds internal.DataSource, pkgPath, v1Path, modulePath string) (*VersionsDetails, error) {
	versions, err := ds.LegacyGetTaggedVersionsForPackageSeries(ctx, pkgPath)
	if err != nil {
		return nil, err
	}
	// If no tagged versions for the package series are found, fetch the
	// pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = ds.LegacyGetPsuedoVersionsForPackageSeries(ctx, pkgPath)
		if err != nil {
			return nil, err
		}
	}

	linkify := func(mi *internal.ModuleInfo) string {
		// Here we have only version information, but need to construct the full
		// import path of the package corresponding to this version.
		var versionPath string
		if mi.ModulePath == stdlib.ModulePath {
			versionPath = pkgPath
		} else {
			versionPath = pathInVersion(v1Path, mi)
		}
		return constructPackageURL(versionPath, mi.ModulePath, linkVersion(mi.Version, mi.ModulePath))
	}
	return buildVersionDetails(modulePath, versions, linkify), nil
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
	suffix := strings.TrimPrefix(strings.TrimPrefix(v1Path, mi.SeriesPath()), "/")
	if suffix == "" {
		return mi.ModulePath
	}
	return path.Join(mi.ModulePath, suffix)
}

// buildVersionDetails constructs the version hierarchy to be rendered on the
// versions tab, organizing major versions into those that have the same module
// path as the package version under consideration, and those that don't.  The
// given versions MUST be sorted first by module path and then by semver.
func buildVersionDetails(currentModulePath string, modInfos []*internal.ModuleInfo, linkify func(v *internal.ModuleInfo) string) *VersionsDetails {

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
			} else if major != "v0" && !strings.HasPrefix(major, "go") {
				major = "v1"
			}
		}
		key := VersionListKey{ModulePath: mi.ModulePath, Major: major}
		vs := &VersionSummary{
			Link:       linkify(mi),
			CommitTime: elapsedTime(mi.CommitTime),
			Version:    linkVersion(mi.Version, mi.ModulePath),
		}
		if _, ok := lists[key]; !ok {
			seenLists = append(seenLists, key)
		}
		lists[key] = append(lists[key], vs)
	}

	var details VersionsDetails
	for _, key := range seenLists {
		vl := &VersionList{
			VersionListKey: key,
			Versions:       lists[key],
		}
		if key.ModulePath == currentModulePath {
			details.ThisModule = append(details.ThisModule, vl)
		} else {
			details.OtherModules = append(details.OtherModules, vl)
		}
	}
	return &details
}

// formatVersion formats a more readable representation of the given version
// string. On any parsing error, it simply returns the input unmodified.
//
// For prerelease versions, formatVersion separates the prerelease portion of
// the version string into a parenthetical. i.e.
//   formatVersion("v1.2.3-alpha") = "v1.2.3 (alpha)"
//
// For pseudo versions, formatVersion uses a short commit hash to identify the
// version. i.e.
//   formatVersion("v1.2.3-20190311183353-d8887717615a") = "v.1.2.3 (d888771)"
func formatVersion(v string) string {
	vType, err := version.ParseType(v)
	if err != nil {
		log.Errorf(context.TODO(), "Error parsing version %q: %v", v, err)
		return v
	}
	pre := semver.Prerelease(v)
	base := strings.TrimSuffix(v, pre)
	pre = strings.TrimPrefix(pre, "-")
	switch vType {
	case version.TypePrerelease:
		return fmt.Sprintf("%s (%s)", base, pre)
	case version.TypePseudo:
		rev := pseudoVersionRev(v)
		commitLen := 7
		if len(rev) < commitLen {
			commitLen = len(rev)
		}
		return fmt.Sprintf("%s (%s)", base, rev[0:commitLen])
	default:
		return v
	}
}

// pseudoVersionRev extracts the commit identifier from a pseudo version
// string. It assumes the pseudo version is correctly formatted.
func pseudoVersionRev(v string) string {
	v = strings.TrimSuffix(v, "+incompatible")
	j := strings.LastIndex(v, "-")
	return v[j+1:]
}

// displayVersion returns the version string, formatted for display.
func displayVersion(v string, modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return goTagForVersion(v)
	}
	return formatVersion(v)
}

// linkVersion returns the version string, suitable for use in
// a link to this site.
func linkVersion(v string, modulePath string) string {
	if modulePath == stdlib.ModulePath {
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
