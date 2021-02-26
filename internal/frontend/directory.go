// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
)

// UnitDirectory is the union of nested modules and subdirectories for a
// unit organized in a two level tree structure. This content is used in the
// directories section of the unit page.
type UnitDirectory struct {
	// Prefix is the prefix of the unit path for the subdirectories.
	Prefix string

	// Root is the package located at prefix, nil for a directory.
	Root *Subdirectory

	// Subdirectories contains subdirectories with prefix trimmed from their suffix.
	Subdirectories []*Subdirectory
}

// NestedModule is a nested module relative to the path of a given unit.
// This content is used in the Directories section of the unit page.
type NestedModule struct {
	Suffix string // suffix after the unit path
	URL    string
}

// Subdirectory is a package in a subdirectory relative to the path of a given
// unit. This content is used in the Directories section of the unit page.
type Subdirectory struct {
	Suffix   string
	URL      string
	Synopsis string
	IsModule bool
}

// unitDirectories zips the subdirectories and nested modules together in a two
// level tree hierarchy.
func unitDirectories(dirs []*Subdirectory, mods []*NestedModule) []*UnitDirectory {
	var merged []*Subdirectory
	for _, d := range dirs {
		merged = append(merged, &Subdirectory{Suffix: d.Suffix,
			Synopsis: d.Synopsis, URL: d.URL, IsModule: false})
	}
	for _, m := range mods {
		merged = append(merged, &Subdirectory{Suffix: m.Suffix, URL: m.URL, IsModule: true})
	}

	// Organize the subdirectories into a two level tree hierarchy. The first part of
	// the unit path suffix for a subdirectory becomes the prefix under which matching
	// subdirectories are grouped.
	mappedDirs := make(map[string]*UnitDirectory)
	for _, d := range merged {
		prefix := strings.Split(d.Suffix, "/")[0]
		if _, ok := mappedDirs[prefix]; !ok {
			mappedDirs[prefix] = &UnitDirectory{Prefix: prefix}
		}
		d.Suffix = strings.TrimPrefix(d.Suffix, prefix+"/")
		if prefix == d.Suffix {
			mappedDirs[prefix].Root = d
		} else {
			mappedDirs[prefix].Subdirectories = append(mappedDirs[prefix].Subdirectories, d)
		}
	}

	var sorted []*UnitDirectory
	for _, p := range mappedDirs {
		sorted = append(sorted, p)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Prefix < sorted[j].Prefix })
	return sorted
}

func getNestedModules(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, sds []*Subdirectory) ([]*NestedModule, error) {
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
	var mods []*NestedModule
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
		mods = append(mods, &NestedModule{
			URL:    constructUnitURL(m.ModulePath, m.ModulePath, internal.LatestVersion),
			Suffix: suffix,
		})
	}
	return mods, nil
}

func getSubdirectories(um *internal.UnitMeta, pkgs []*internal.PackageMeta) []*Subdirectory {
	var sdirs []*Subdirectory
	for _, pm := range pkgs {
		if um.Path == pm.Path {
			continue
		}
		sdirs = append(sdirs, &Subdirectory{
			URL:      constructUnitURL(pm.Path, um.ModulePath, linkVersion(um.Version, um.ModulePath)),
			Suffix:   internal.Suffix(pm.Path, um.Path),
			Synopsis: pm.Synopsis,
		})
	}
	sort.Slice(sdirs, func(i, j int) bool { return sdirs[i].Suffix < sdirs[j].Suffix })
	return sdirs
}
