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

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/discovery/internal/version"
)

// VersionsDetails contains the hierarchy of version summary information used
// to populate the version tab.
type VersionsDetails struct {
	// ThisModule is the slice of MajorVersionGroups with the same module path as
	// the current package.
	ThisModule []*MajorVersionGroup

	// OtherModules is the slice of MajorVersionGroups with a different module
	// path from the current package.
	OtherModules []*MajorVersionGroup
}

// MajorVersionGroup holds all versions corresponding to a unique (module path,
// major version) tuple in the version hierarchy. Notably v0 and v1 module
// versions are in separate MajorVersionGroups, despite having the same module
// path.
type MajorVersionGroup struct {
	// Major is the major version string (e.g. v1, v2)
	Major      string
	ModulePath string
	// Versions holds the nested version summaries, organized in descending minor
	// and patch version order.
	Versions [][]*VersionSummary
}

// VersionSummary holds data required to format the version link on the
// versions tab.
type VersionSummary struct {
	Version          string
	CommitTime       string
	FormattedVersion string
	// Link to this version, for use in the anchor href.
	Link string
}

// fetchModuleVersionsDetails builds a version hierarchy for module versions
// with the same series path as the given version.
func fetchModuleVersionsDetails(ctx context.Context, ds DataSource, vi *internal.VersionInfo) (*VersionsDetails, error) {
	versions, err := ds.GetTaggedVersionsForModule(ctx, vi.ModulePath)
	if err != nil {
		return nil, err
	}
	// If no tagged versions of the module are found, fetch pseudo-versions
	// instead.
	if len(versions) == 0 {
		versions, err = ds.GetPseudoVersionsForModule(ctx, vi.ModulePath)
		if err != nil {
			return nil, err
		}
	}
	linkify := func(v *internal.VersionInfo) string {
		var formattedVersion = v.Version
		if v.ModulePath == stdlib.ModulePath {
			formattedVersion = goTagForVersion(v.Version)
		}
		return constructModuleURL(v.ModulePath, formattedVersion)
	}
	return buildVersionDetails(vi.ModulePath, versions, linkify), nil
}

// fetchPackageVersionsDetails builds a version hierarchy for all module
// versions containing a package path with v1 import path matching the v1
// import path of pkg.
func fetchPackageVersionsDetails(ctx context.Context, ds DataSource, pkg *internal.VersionedPackage) (*VersionsDetails, error) {
	versions, err := ds.GetTaggedVersionsForPackageSeries(ctx, pkg.Path)
	if err != nil {
		return nil, err
	}
	// If no tagged versions for the package series are found, fetch the
	// pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = ds.GetPseudoVersionsForPackageSeries(ctx, pkg.Path)
		if err != nil {
			return nil, err
		}
	}

	var filteredVersions []*internal.VersionInfo
	// TODO(rfindley): remove this filtering, as it should not be necessary and
	// is probably a relic of earlier version query implementations.
	for _, v := range versions {
		if seriesPath := v.SeriesPath(); strings.HasPrefix(pkg.V1Path, seriesPath) || seriesPath == stdlib.ModulePath {
			filteredVersions = append(filteredVersions, v)
		} else {
			log.Errorf("got version with mismatching series: %q", seriesPath)
		}
	}

	linkify := func(vi *internal.VersionInfo) string {
		// Here we have only version information, but need to construct the full
		// import path of the package corresponding to this version.
		var versionPath, formattedVersion string
		if vi.ModulePath == stdlib.ModulePath {
			versionPath = pkg.Path
			formattedVersion = goTagForVersion(vi.Version)
		} else {
			versionPath = pathInVersion(pkg.V1Path, vi)
			formattedVersion = vi.Version
		}
		return constructPackageURL(versionPath, vi.ModulePath, formattedVersion)
	}
	return buildVersionDetails(pkg.ModulePath, filteredVersions, linkify), nil
}

// pathInVersion constructs the full import path of the package corresponding
// to vi, given its v1 path. To do this, we first compute the suffix of the
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
func pathInVersion(v1Path string, vi *internal.VersionInfo) string {
	suffix := strings.TrimPrefix(strings.TrimPrefix(v1Path, vi.SeriesPath()), "/")
	if suffix == "" {
		return vi.ModulePath
	}
	return path.Join(vi.ModulePath, suffix)
}

// buildVersionDetails constructs the version hierarchy to be rendered on the
// versions tab, organizing major versions into those that have the same module
// path as the package version under consideration, and those that don't.
func buildVersionDetails(currentModulePath string, versions []*internal.VersionInfo, linkify func(v *internal.VersionInfo) string) *VersionsDetails {
	// Pre-sort versions to ensure they are in descending semver order.
	sort.Slice(versions, func(i, j int) bool {
		return semver.Compare(versions[i].Version, versions[j].Version) > 0
	})

	// Next, build a version tree containing each unique version path:
	//   modulePath->major->majMin->version
	tree := &versionTree{}
	for _, v := range versions {
		// Try to resolve the most appropriate major version for this version. If
		// we detect a +incompatible version (when the path version does not match
		// the sematic version), we prefer the path version.
		major := semver.Major(v.Version)
		if v.ModulePath == stdlib.ModulePath {
			var err error
			major, err = stdlib.MajorVersionForVersion(v.Version)
			if err != nil {
				panic(err)
			}
		}
		if _, pathMajor, ok := module.SplitPathVersion(v.ModulePath); ok {
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
		majMin := semver.MajorMinor(v.Version)
		tree.push(v, v.ModulePath, major, majMin, v.Version)
	}

	// makeMV builds a MajorVersionGroup from a major subtree of the version
	// hierarchy.
	makeMV := func(modulePath, major string, majorTree *versionTree) *MajorVersionGroup {
		mvg := MajorVersionGroup{
			Major:      major,
			ModulePath: modulePath,
		}
		majorTree.forEach(func(_ string, minorTree *versionTree) {
			patches := []*VersionSummary{}
			minorTree.forEach(func(_ string, patchTree *versionTree) {

				version := patchTree.versionInfo.Version
				formattedVersion := formatVersion(version)
				if patchTree.versionInfo.ModulePath == stdlib.ModulePath {
					formattedVersion = goTagForVersion(patchTree.versionInfo.Version)
					version = formattedVersion
				}
				patches = append(patches, &VersionSummary{
					Version:          version,
					Link:             linkify(patchTree.versionInfo),
					CommitTime:       elapsedTime(patchTree.versionInfo.CommitTime),
					FormattedVersion: formattedVersion,
				})
			})
			mvg.Versions = append(mvg.Versions, patches)
		})
		return &mvg
	}

	// Finally, VersionDetails is built by traversing the major version
	// hierarchy.
	var details VersionsDetails
	tree.forEach(func(modulePath string, moduleTree *versionTree) {
		moduleTree.forEach(func(major string, majorTree *versionTree) {
			mv := makeMV(modulePath, major, majorTree)
			if modulePath == currentModulePath {
				details.ThisModule = append(details.ThisModule, mv)
			} else {
				details.OtherModules = append(details.OtherModules, mv)
			}
		})
	})
	return &details
}

// versionTree represents the version hierarchy. It preserves the order in
// which versions are added.
type versionTree struct {
	// seen tracks the order in which new keys are added.
	seen []string

	// A tree can hold either a nested subtree or a VersionInfo.  When building
	// VersionDetails, it has the following hierarchy
	//	modulePath
	//		major
	//			majorMinor
	//				fullVersion
	subTrees    map[string]*versionTree
	versionInfo *internal.VersionInfo
}

// push adds a new version to the version hierarchy, if it doesn't already
// exist.
func (t *versionTree) push(v *internal.VersionInfo, path ...string) {
	if len(path) == 0 {
		t.versionInfo = v
		return
	}
	if t.subTrees == nil {
		t.subTrees = make(map[string]*versionTree)
	}
	subTree, ok := t.subTrees[path[0]]
	if !ok {
		t.seen = append(t.seen, path[0])
		subTree = &versionTree{}
		t.subTrees[path[0]] = subTree
	}
	subTree.push(v, path[1:]...)
}

// forEach iterates through sub-versionTrees in the version hierarchy, calling
// the given function for each.
func (t *versionTree) forEach(f func(string, *versionTree)) {
	for _, k := range t.seen {
		f(k, t.subTrees[k])
	}
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
		log.Errorf("Error parsing version %q: %v", v, err)
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

// goTagForVersion returns the Go tag corresponding to a given semantic
// version. It should only be used if we are 100% sure the version will
// correspond to a Go tag, such as when we are fetching the version from the

func goTagForVersion(v string) string {
	tag, err := stdlib.TagForVersion(v)
	if err != nil {
		panic(fmt.Errorf("BUG: unable to get tag for version: %q", err))
	}
	return tag
}
