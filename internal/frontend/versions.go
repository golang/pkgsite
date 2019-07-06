// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// VersionsDetails contains all the data that the versions tab
// template needs to populate.
type VersionsDetails struct {
	// ThisMajorVersion is the MajorVersionGroup containing the package currently
	// being viewed.
	ThisModule []*ModuleVersion

	// OtherMajorVersions is other major module versions in the current module
	// series.
	OtherModules []*ModuleVersion
}

// ModuleVersion holds all versions within a unique module path in the version hierarchy.
type ModuleVersion struct {
	Major      string
	ModulePath string
	Versions   [][]*PackageVersion
}

// PackageVersion represents the patch level of the versions hierarchy.
type PackageVersion struct {
	Version          string
	Path             string
	CommitTime       string
	FormattedVersion string
}

// fetchVersionsDetails fetches data for the module version specified by path
// and version from the database and returns a VersionsDetails. The package
// path for each SeriesVersionGroup is calculated as the module path and
// package suffix. For the standard library packages, it is the package path.
func fetchVersionsDetails(ctx context.Context, db *postgres.DB, pkg *internal.VersionedPackage) (*VersionsDetails, error) {
	versions, err := db.GetTaggedVersionsForPackageSeries(ctx, pkg.Path)
	if err != nil {
		return nil, fmt.Errorf("db.GetTaggedVersions(%q): %v", pkg.Path, err)
	}
	// If no tagged versions for the package series are found,
	// fetch the pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = db.GetPseudoVersionsForPackageSeries(ctx, pkg.Path)
		if err != nil {
			return nil, fmt.Errorf("db.GetPseudoVersions(%q): %v", pkg.Path, err)
		}
	}

	// Next, build a versionTree containing all valid versions.
	tree := &versionTree{}
	for _, v := range versions {
		if vs := v.SeriesPath(); !strings.HasPrefix(pkg.V1Path, vs) && !internal.IsStandardLibraryModule(vs) {
			log.Printf("got version with mismatching series: %q", vs)
			continue
		}
		major := semver.Major(v.Version)
		majMin := semver.MajorMinor(v.Version)
		tree.push(v, v.ModulePath, major, majMin, v.Version)
	}

	// versionPath backs-out the package path corresponding to a package version,
	// by computing the relative path to the package within the module version.
	versionPath := func(v *internal.VersionInfo) string {
		if inStdLib(pkg.Path) {
			return pkg.Path
		}
		suffix := strings.TrimPrefix(strings.TrimPrefix(pkg.V1Path, v.SeriesPath()), "/")
		if suffix == "" {
			return v.ModulePath
		}
		return path.Join(v.ModulePath, suffix)
	}

	// makeMV builds a MajorVersionGroup from a major subtree of the version
	// hierarchy.
	makeMV := func(modulePath, major string, majorTree *versionTree) *ModuleVersion {
		mvg := ModuleVersion{
			Major:      major,
			ModulePath: modulePath,
		}
		majorTree.forEach(func(_ string, minorTree *versionTree) {
			patches := []*PackageVersion{}
			minorTree.forEach(func(version string, patchTree *versionTree) {
				patches = append(patches, &PackageVersion{
					Version:          patchTree.versionInfo.Version,
					Path:             versionPath(patchTree.versionInfo),
					CommitTime:       elapsedTime(patchTree.versionInfo.CommitTime),
					FormattedVersion: formatVersion(patchTree.versionInfo.Version),
				})
			})
			mvg.Versions = append(mvg.Versions, patches)
		})
		return &mvg
	}

	// Finally, VersionDetails is built by traversing the version hierarchy using
	// the helper functions defined above.
	var details VersionsDetails
	tree.forEach(func(modulePath string, moduleTree *versionTree) {
		moduleTree.forEach(func(major string, majorTree *versionTree) {
			mv := makeMV(modulePath, major, majorTree)
			if modulePath == pkg.ModulePath {
				details.ThisModule = append(details.ThisModule, mv)
			} else {
				details.OtherModules = append(details.OtherModules, mv)
			}
		})
	})
	return &details, nil
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
func formatVersion(version string) string {
	// TODO(b/136649901): move ParseVersionType to a better location.
	vType, err := etl.ParseVersionType(version)
	if err != nil {
		log.Printf("Error parsing version %q: %v", version, err)
		return version
	}
	pre := semver.Prerelease(version)
	base := strings.TrimSuffix(version, pre)
	pre = strings.TrimPrefix(pre, "-")
	switch vType {
	case internal.VersionTypePrerelease:
		return fmt.Sprintf("%s (%s)", base, pre)
	case internal.VersionTypePseudo:
		rev := pseudoVersionRev(version)
		commitLen := 7
		if len(rev) < commitLen {
			commitLen = len(rev)
		}
		return fmt.Sprintf("%s (%s)", base, rev[0:commitLen])
	default:
		return version
	}
}

// pseudoVersionRev extracts the commit identifier from a pseudo version
// string. It assumes the pseudo version is correctly formatted.
func pseudoVersionRev(v string) string {
	v = strings.TrimSuffix(v, "+incompatible")
	j := strings.LastIndex(v, "-")
	return v[j+1:]
}
