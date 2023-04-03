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
	Suffix     string
	URL        string
	Synopsis   string
	IsModule   bool
	IsInternal bool
}

// unitDirectories zips the subdirectories and nested modules together in a two
// level tree hierarchy.
func unitDirectories(directories []*DirectoryInfo) []*Directory {
	if len(directories) == 0 {
		return nil
	}
	// Organize the subdirectories into a two level tree hierarchy. The first part of
	// the unit path suffix for a subdirectory becomes the prefix under which matching
	// subdirectories are grouped.
	mappedDirs := make(map[string]*Directory)
	for _, d := range directories {
		prefix, _, _ := strings.Cut(d.Suffix, "/")

		// Mark internal directories as internal. They are hidden by default and made visible
		// by clicking a toggle button.
		if strings.HasPrefix(d.Suffix, "internal/") ||
			strings.HasSuffix(d.Suffix, "/internal") ||
			strings.Contains(d.Suffix, "/internal/") {
			d.IsInternal = true
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

	var dirs []*Directory
	for _, dir := range mappedDirs {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Prefix < dirs[j].Prefix
	})
	return dirs
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
