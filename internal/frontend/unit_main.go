// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/version"
)

// MainDetails contains data needed to render the unit template.
type MainDetails struct {
	// NestedModules are nested modules relative to the path for the unit.
	NestedModules []*NestedModule

	// Subdirectories are packages in subdirectories relative to the path for
	// the unit.
	Subdirectories []*Subdirectory

	// Directories are packages and nested modules relative to the path for the
	// unit.
	Directories []*UnitDirectory

	// Licenses contains license metadata used in the header.
	Licenses []LicenseMetadata

	// NumImports is the number of imports for the package.
	NumImports int

	// CommitTime is time that this version was published, or the time that
	// has elapsed since this version was committed if it was done so recently.
	CommitTime string

	// Readme is the rendered readme HTML.
	Readme safehtml.HTML

	// ReadmeOutline is a collection of headings from the readme file
	// used to render the readme outline in the sidebar.
	ReadmeOutline []*Heading

	// ReadmeLinks are from the "Links" section of this unit's readme file, and
	// are displayed on the right sidebar.
	ReadmeLinks []link

	// DocLinks are from the "Links" section of the Go package documentation,
	// and are displayed on the right sidebar.
	DocLinks []link

	// ModuleReadmeLinks are from the "Links" section of this unit's module, if
	// the unit is not itself a module. They are displayed on the right sidebar.
	// See https://golang.org/issue/42968.
	ModuleReadmeLinks []link

	// ImportedByCount is the number of packages that import this path.
	// When the count is > limit it will read as 'limit+'. This field
	// is not supported when using a datasource proxy.
	ImportedByCount int

	DocBody       safehtml.HTML
	DocOutline    safehtml.HTML
	MobileOutline safehtml.HTML
	IsPackage     bool

	// DocSynopsis is used as the content for the <meta name="Description">
	// tag on the main unit page.
	DocSynopsis string

	// SourceFiles contains .go files for the package.
	SourceFiles []*File

	// RepositoryURL is the URL to the repository containing the package.
	RepositoryURL string

	// SourceURL is the URL to the source of the package.
	SourceURL string

	// ExpandReadme is holds the expandable readme state.
	ExpandReadme bool

	// ModFileURL is an URL to the mod file.
	ModFileURL string

	// IsTaggedVersion is true if the version is not a psuedorelease.
	IsTaggedVersion bool

	// IsStableVersion is true if the major version is v1 or greater.
	IsStableVersion bool
}

// File is a source file for a package.
type File struct {
	Name string
	URL  string
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

func fetchMainDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, expandReadme bool) (_ *MainDetails, err error) {
	defer middleware.ElapsedStat(ctx, "fetchMainDetails")()

	unit, err := ds.GetUnit(ctx, um, internal.WithMain)
	if err != nil {
		return nil, err
	}
	subdirectories := getSubdirectories(um, unit.Subdirectories)
	if err != nil {
		return nil, err
	}
	nestedModules, err := getNestedModules(ctx, ds, um, subdirectories)
	if err != nil {
		return nil, err
	}
	readme, err := readmeContent(ctx, unit)
	if err != nil {
		return nil, err
	}
	var (
		docParts           = &dochtml.Parts{}
		docLinks, modLinks []link
		files              []*File
		synopsis           string
	)
	if unit.Documentation != nil {
		synopsis = unit.Documentation[0].Synopsis
		end := middleware.ElapsedStat(ctx, "DecodePackage")
		docPkg, err := godoc.DecodePackage(unit.Documentation[0].Source)
		end()
		if err != nil {
			if errors.Is(err, godoc.ErrInvalidEncodingType) {
				// Instead of returning a 500, return a 404 so the user can
				// reprocess the documentation.
				log.Errorf(ctx, "fetchMainDetails(%q, %q, %q): %v", um.Path, um.ModulePath, um.Version, err)
				return nil, errUnitNotFoundWithoutFetch
			}
			return nil, err
		}
		docParts, err = getHTML(ctx, unit, docPkg)
		// If err  is ErrTooLarge, then docBody will have an appropriate message.
		if err != nil && !errors.Is(err, dochtml.ErrTooLarge) {
			return nil, err
		}
		for _, l := range docParts.Links {
			docLinks = append(docLinks, link{Href: l.Href, Body: l.Text})
		}
		end = middleware.ElapsedStat(ctx, "sourceFiles")
		files = sourceFiles(unit, docPkg)
		end()
	}
	// If the unit is not a module, fetch the module readme to extract its
	// links.
	// In the unlikely event that the module is redistributable but the unit is
	// not, we will not show the module links on the unit page.
	if unit.Path != unit.ModulePath && unit.IsRedistributable {
		modReadme, err := ds.GetModuleReadme(ctx, unit.ModulePath, unit.Version)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		if err == nil {
			rm, err := processReadme(modReadme, um.SourceInfo)
			if err != nil {
				return nil, err
			}
			modLinks = rm.Links
		}
	}

	versionType, err := version.ParseType(um.Version)
	if err != nil {
		return nil, err
	}
	isTaggedVersion := versionType != version.TypePseudo

	return &MainDetails{
		ExpandReadme:      expandReadme,
		NestedModules:     nestedModules,
		Subdirectories:    subdirectories,
		Directories:       unitDirectories(subdirectories, nestedModules),
		Licenses:          transformLicenseMetadata(um.Licenses),
		CommitTime:        absoluteTime(um.CommitTime),
		Readme:            readme.HTML,
		ReadmeOutline:     readme.Outline,
		ReadmeLinks:       readme.Links,
		DocLinks:          docLinks,
		ModuleReadmeLinks: modLinks,
		DocOutline:        docParts.Outline,
		DocBody:           docParts.Body,
		DocSynopsis:       synopsis,
		SourceFiles:       files,
		RepositoryURL:     um.SourceInfo.RepoURL(),
		SourceURL:         um.SourceInfo.DirectoryURL(internal.Suffix(um.Path, um.ModulePath)),
		MobileOutline:     docParts.MobileOutline,
		NumImports:        unit.NumImports,
		ImportedByCount:   unit.NumImportedBy,
		IsPackage:         unit.IsPackage(),
		ModFileURL:        um.SourceInfo.ModuleURL() + "/go.mod",
		IsTaggedVersion:   isTaggedVersion,
		IsStableVersion:   semver.Major(um.Version) != "v0",
	}, nil
}

// readmeContent renders the readme to html and collects the headings
// into an outline.
func readmeContent(ctx context.Context, u *internal.Unit) (_ *Readme, err error) {
	defer derrors.Wrap(&err, "readmeContent(%q, %q, %q)", u.Path, u.ModulePath, u.Version)
	defer middleware.ElapsedStat(ctx, "readmeContent")()
	if !u.IsRedistributable {
		return &Readme{}, nil
	}
	return ProcessReadme(ctx, u)
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

const missingDocReplacement = `<p>Documentation is missing.</p>`

func getHTML(ctx context.Context, u *internal.Unit, docPkg *godoc.Package) (_ *dochtml.Parts, err error) {
	defer derrors.Wrap(&err, "getHTML(%s)", u.Path)

	if len(u.Documentation[0].Source) > 0 {
		return renderDocParts(ctx, u, docPkg)
	}
	log.Errorf(ctx, "unit %s (%s@%s) missing documentation source", u.Path, u.ModulePath, u.Version)
	return &dochtml.Parts{Body: template.MustParseAndExecuteToHTML(missingDocReplacement)}, nil
}
