// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
)

// MainDetails contains data needed to render the unit template.
type MainDetails struct {
	// NestedModules are nested modules relative to the path for the unit.
	NestedModules []*NestedModule

	// Subdirectories are packages in subdirectories relative to the path for
	// the unit.
	Subdirectories []*Subdirectory

	// Licenses contains license metadata used in the header.
	Licenses []LicenseMetadata

	// NumImports is the number of imports for the package.
	NumImports int

	// CommitTime is time that this version was published, or the time that
	// has elapsed since this version was committed if it was done so recently.
	CommitTime string

	// Readme is the rendered readme HTML.
	Readme safehtml.HTML

	// ImportedByCount is the number of packages that import this path.
	// When the count is > limit it will read as 'limit+'. This field
	// is not supported when using a datasource proxy.
	ImportedByCount string

	DocBody       safehtml.HTML
	DocOutline    safehtml.HTML
	MobileOutline safehtml.HTML
	IsPackage     bool

	// SourceFiles contains .go files for the package.
	SourceFiles []*File

	// RepositoryURL is the URL to the repository containing the package.
	RepositoryURL string

	// SourceURL is the URL to the source of the package.
	SourceURL string

	// ExpandReadme is holds the expandable readme state.
	ExpandReadme bool
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
}

func fetchMainDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, expandReadme bool) (_ *MainDetails, err error) {
	defer middleware.ElapsedStat(ctx, "fetchMainDetails")()

	unit, err := ds.GetUnit(ctx, um, internal.WithReadme|internal.WithDocumentation|internal.WithSubdirectories|internal.WithImports)
	if err != nil {
		return nil, err
	}

	importedByCount := strconv.Itoa(unit.NumImportedBy)
	if !experiment.IsActive(ctx, internal.ExperimentGetUnitWithOneQuery) {
		// importedByCount is not supported when using a datasource proxy.
		importedByCount = "0"
		db, ok := ds.(*postgres.DB)
		if ok {
			importedBy, err := db.GetImportedBy(ctx, um.Path, um.ModulePath, importedByLimit)
			if err != nil {
				return nil, err
			}
			// If we reached the query limit, then we don't know the total
			// and we'll indicate that with a '+'. For example, if the limit
			// is 101 and we get 101 results, then we'll show '100+ Imported by'.
			importedByCount = strconv.Itoa(len(importedBy))
			if len(importedBy) == importedByLimit {
				importedByCount = strconv.Itoa(len(importedBy)-1) + "+"
			}
		}
	}

	nestedModules, err := getNestedModules(ctx, ds, um)
	if err != nil {
		return nil, err
	}
	subdirectories := getSubdirectories(um, unit.Subdirectories)
	if err != nil {
		return nil, err
	}
	readme, err := readmeContent(ctx, um, unit.Readme)
	if err != nil {
		return nil, err
	}

	var (
		docBody, docOutline, mobileOutline safehtml.HTML
		files                              []*File
	)
	if unit.Documentation != nil {
		end := middleware.ElapsedStat(ctx, "DecodePackage")
		docPkg, err := godoc.DecodePackage(unit.Documentation.Source)
		end()
		if err != nil {
			return nil, err
		}
		docHTML := getHTML(ctx, unit, docPkg)
		// TODO: Deprecate godoc.Parse. The sidenav and body can
		// either be rendered using separate functions, or all this content can
		// be passed to the template via the UnitPage struct.
		end = middleware.ElapsedStat(ctx, "godoc Parses")
		b, err := godoc.Parse(docHTML, godoc.BodySection)
		if err != nil {
			return nil, err
		}
		docBody = b
		o, err := godoc.Parse(docHTML, godoc.SidenavSection)
		if err != nil {
			return nil, err
		}
		docOutline = o
		m, err := godoc.Parse(docHTML, godoc.SidenavMobileSection)
		if err != nil {
			return nil, err
		}
		mobileOutline = m
		end()

		end = middleware.ElapsedStat(ctx, "sourceFiles")
		files = sourceFiles(unit, docPkg)
		end()
		if err != nil {
			return nil, err
		}
	}
	return &MainDetails{
		ExpandReadme:    expandReadme,
		NestedModules:   nestedModules,
		Subdirectories:  subdirectories,
		Licenses:        transformLicenseMetadata(um.Licenses),
		CommitTime:      elapsedTime(um.CommitTime),
		Readme:          readme,
		DocOutline:      docOutline,
		DocBody:         docBody,
		SourceFiles:     files,
		RepositoryURL:   um.SourceInfo.RepoURL(),
		SourceURL:       um.SourceInfo.DirectoryURL(internal.Suffix(um.Path, um.ModulePath)),
		MobileOutline:   mobileOutline,
		NumImports:      len(unit.Imports),
		ImportedByCount: importedByCount,
		IsPackage:       unit.IsPackage(),
	}, nil
}

// moduleInfo extracts module info from a unit. This is a shim
// for functions ReadmeHTML and createDirectory that will be removed
// when we complete the switch to units.
func moduleInfo(um *internal.UnitMeta) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
		SourceInfo:        um.SourceInfo,
	}
}

// readmeContent renders the readme to html.
func readmeContent(ctx context.Context, um *internal.UnitMeta, readme *internal.Readme) (safehtml.HTML, error) {
	defer middleware.ElapsedStat(ctx, "readmeContent")()

	if um.IsRedistributable && readme != nil {
		mi := moduleInfo(um)
		readme, err := ReadmeHTML(ctx, mi, readme)
		if err != nil {
			return safehtml.HTML{}, err
		}
		return readme, nil
	}
	return safehtml.HTML{}, nil
}

func getNestedModules(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) ([]*NestedModule, error) {
	nestedModules, err := ds.GetNestedModules(ctx, um.ModulePath)
	if err != nil {
		return nil, err
	}
	var mods []*NestedModule
	for _, m := range nestedModules {
		if m.SeriesPath() == internal.SeriesPathForModule(um.ModulePath) {
			continue
		}
		if !strings.HasPrefix(m.ModulePath, um.Path+"/") {
			continue
		}
		mods = append(mods, &NestedModule{
			URL:    constructPackageURL(m.ModulePath, m.ModulePath, internal.LatestVersion),
			Suffix: internal.Suffix(m.SeriesPath(), um.Path),
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
			URL:      constructPackageURL(pm.Path, um.ModulePath, linkVersion(um.Version, um.ModulePath)),
			Suffix:   internal.Suffix(pm.Path, um.Path),
			Synopsis: pm.Synopsis,
		})
	}
	sort.Slice(sdirs, func(i, j int) bool { return sdirs[i].Suffix < sdirs[j].Suffix })
	return sdirs
}

func getHTML(ctx context.Context, u *internal.Unit, docPkg *godoc.Package) safehtml.HTML {
	if experiment.IsActive(ctx, internal.ExperimentFrontendRenderDoc) && len(u.Documentation.Source) > 0 {
		dd, err := renderDoc(ctx, u, docPkg)
		if err != nil {
			log.Errorf(ctx, "render doc failed: %v", err)
			// Fall through to use stored doc.
		} else {
			return dd.Documentation
		}
	}
	return u.Documentation.HTML
}
