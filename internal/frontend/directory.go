// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// Directories is the directory listing for all directories in the unit,
// which is listed in the directories section of the main page.
type Directories struct {
	// External contains all of the non-internal directories for the unit.
	External []*Directory

	// Internal contains the top level internal directory for the unit, if any.
	Internal *Directory
}

// Directory is either a nested module or subdirectory of a unit, organized in
// a two level tree structure. This content is used in the
// directories section of the unit page.
type Directory struct {
	// Prefix is the prefix of the unit path for the subdirectories.
	Prefix string

	// Root is the package located at prefix, nil for a directory.
	Root *DirectoryInfo

	// Subdirectories contains subdirectories with prefix trimmed from their suffix.
	Subdirectories []*DirectoryInfo
}

// DirectoryInfo contains information about a package or nested module,
// relative to the path of a given unit. This content is used in the
// Directories section of the unit page.
type DirectoryInfo struct {
	Suffix   string
	URL      string
	Synopsis string
	IsModule bool
}

// unitDirectories zips the subdirectories and nested modules together in a two
// level tree hierarchy.
func unitDirectories(directories []*DirectoryInfo) *Directories {
	if len(directories) == 0 {
		return nil
	}
	// Organize the subdirectories into a two level tree hierarchy. The first part of
	// the unit path suffix for a subdirectory becomes the prefix under which matching
	// subdirectories are grouped.
	mappedDirs := make(map[string]*Directory)
	for _, d := range directories {
		prefix, _, _ := strings.Cut(d.Suffix, "/")

		// Skip internal directories that are not in the top level internal
		// directory. For example, foo/internal and foo/internal/bar should
		// be skipped, but internal/foo should be included.
		if prefix != "internal" && (strings.HasSuffix(d.Suffix, "/internal") ||
			strings.Contains(d.Suffix, "/internal/")) {
			continue
		}
		if _, ok := mappedDirs[prefix]; !ok {
			mappedDirs[prefix] = &Directory{Prefix: prefix}
		}
		d.Suffix = strings.TrimPrefix(d.Suffix, prefix+"/")
		if prefix == d.Suffix {
			mappedDirs[prefix].Root = d
		} else {
			mappedDirs[prefix].Subdirectories = append(mappedDirs[prefix].Subdirectories, d)
		}
	}

	section := &Directories{}
	for prefix, dir := range mappedDirs {
		if prefix == "internal" {
			section.Internal = dir
		} else {
			section.External = append(section.External, dir)
		}
	}
	sort.Slice(section.External, func(i, j int) bool {
		return section.External[i].Prefix < section.External[j].Prefix
	})
	return section
}

func getNestedModules(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, sds []*DirectoryInfo) ([]*DirectoryInfo, error) {
	nestedModules, err := ds.GetNestedModules(ctx, um.ModulePath)
	if err != nil {
		return nil, err
	}
	// Build a map of existing suffixes in subdirectories to filter out nested modules
	// which have the same suffix.
	excludedSuffixes := make(map[string]bool)
	for _, dir := range sds {
		excludedSuffixes[dir.Suffix] = true
	}
	var mods []*DirectoryInfo
	for _, m := range nestedModules {
		if m.SeriesPath() == internal.SeriesPathForModule(um.ModulePath) {
			continue
		}
		if !strings.HasPrefix(m.ModulePath, um.Path+"/") {
			continue
		}
		suffix := internal.Suffix(m.SeriesPath(), um.Path)
		if excludedSuffixes[suffix] {
			continue
		}
		mods = append(mods, &DirectoryInfo{
			URL:      constructUnitURL(m.ModulePath, m.ModulePath, version.Latest),
			Suffix:   suffix,
			IsModule: true,
		})
	}
	return mods, nil
}

func getSubdirectories(um *internal.UnitMeta, pkgs []*internal.PackageMeta, requestedVersion string) []*DirectoryInfo {
	var sdirs []*DirectoryInfo
	for _, pm := range pkgs {
		if um.Path == pm.Path {
			continue
		}
		if um.Path == stdlib.ModulePath && strings.HasPrefix(pm.Path, "cmd/") {
			// Omit "cmd" from the directory listing on
			// pkg.go.dev/std, since go list std does not
			// list them.
			continue
		}
		sdirs = append(sdirs, &DirectoryInfo{
			URL: constructUnitURL(pm.Path, um.ModulePath,
				linkVersion(um.ModulePath, requestedVersion, um.Version)),
			Suffix:   internal.Suffix(pm.Path, um.Path),
			Synopsis: pm.Synopsis,
		})
	}
	sort.Slice(sdirs, func(i, j int) bool { return sdirs[i].Suffix < sdirs[j].Suffix })
	return sdirs
}
