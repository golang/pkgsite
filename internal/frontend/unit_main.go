// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
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

	// ReadmeOutline is a collection of headings from the readme file
	// used to render the readme outline in the sidebar.
	ReadmeOutline []*Heading

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

	nestedModules, err := getNestedModules(ctx, ds, um)
	if err != nil {
		return nil, err
	}
	subdirectories := getSubdirectories(um, unit.Subdirectories)
	if err != nil {
		return nil, err
	}
	readme, readmeOutline, err := readmeContent(ctx, unit)
	if err != nil {
		return nil, err
	}
	importedByCount, err := getImportedByCount(ctx, ds, unit)
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
			if errors.Is(err, godoc.ErrInvalidEncodingType) {
				// Instead of returning a 500, return a 404 so the user can
				// reprocess the documentation.
				log.Errorf(ctx, "fetchMainDetails(%q, %q, %q): %v", um.Path, um.ModulePath, um.Version, err)
				return nil, errUnitNotFoundWithoutFetch
			}
			return nil, err
		}
		docBody, docOutline, mobileOutline, err = getHTML(ctx, unit, docPkg)
		if err != nil {
			return nil, err
		}
		end = middleware.ElapsedStat(ctx, "sourceFiles")
		files = sourceFiles(unit, docPkg)
		end()
	}
	return &MainDetails{
		ExpandReadme:    expandReadme,
		NestedModules:   nestedModules,
		Subdirectories:  subdirectories,
		Licenses:        transformLicenseMetadata(um.Licenses),
		CommitTime:      absoluteTime(um.CommitTime),
		Readme:          readme,
		ReadmeOutline:   readmeOutline,
		DocOutline:      docOutline,
		DocBody:         docBody,
		SourceFiles:     files,
		RepositoryURL:   um.SourceInfo.RepoURL(),
		SourceURL:       um.SourceInfo.DirectoryURL(internal.Suffix(um.Path, um.ModulePath)),
		MobileOutline:   mobileOutline,
		NumImports:      unit.NumImports,
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

// readmeContent renders the readme to html and collects the headings
// into an outline when the goldmark experiment active.
func readmeContent(ctx context.Context, u *internal.Unit) (_ safehtml.HTML, _ []*Heading, err error) {
	defer derrors.Wrap(&err, "readmeContent(%q, %q, %q)", u.Path, u.ModulePath, u.Version)
	defer middleware.ElapsedStat(ctx, "readmeContent")()
	if !u.IsRedistributable {
		return safehtml.HTML{}, nil, nil
	}
	mi := moduleInfo(&u.UnitMeta)
	var (
		readmeHTML    safehtml.HTML
		readmeOutline []*Heading
	)
	if experiment.IsActive(ctx, internal.ExperimentGoldmark) {
		readmeHTML, readmeOutline, err = Readme(ctx, u)
	} else {
		readmeHTML, err = LegacyReadmeHTML(ctx, mi, u.Readme)
	}
	if err != nil {
		return safehtml.HTML{}, nil, err
	}
	return readmeHTML, readmeOutline, nil
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
			URL:    constructUnitURL(m.ModulePath, m.ModulePath, internal.LatestVersion),
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
			URL:      constructUnitURL(pm.Path, um.ModulePath, linkVersion(um.Version, um.ModulePath)),
			Suffix:   internal.Suffix(pm.Path, um.Path),
			Synopsis: pm.Synopsis,
		})
	}
	sort.Slice(sdirs, func(i, j int) bool { return sdirs[i].Suffix < sdirs[j].Suffix })
	return sdirs
}

func getHTML(ctx context.Context, u *internal.Unit, docPkg *godoc.Package) (body, outline, mobileOutline safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "getHTML(%s)", u.Path)

	if experiment.IsActive(ctx, internal.ExperimentFrontendRenderDoc) && len(u.Documentation.Source) > 0 {
		return renderDocParts(ctx, u, docPkg)
	}
	return godoc.ParseDoc(ctx, u.Documentation.HTML)
}

// getImportedByCount fetches the imported by count for the unit and returns a
// string to be displayed. If the datasource does not support imported by, it
// will return N/A.
func getImportedByCount(ctx context.Context, ds internal.DataSource, unit *internal.Unit) (_ string, err error) {
	defer derrors.Wrap(&err, "getImportedByCount(%q, %q, %q)", unit.Path, unit.ModulePath, unit.Version)
	defer middleware.ElapsedStat(ctx, "getImportedByCount")()

	db, ok := ds.(*postgres.DB)
	if !ok {
		return "N/A", nil
	}

	// Get an exact number for a small limit, to determine whether we should
	// fetch data from search_documents and display an approximate count, or
	// just use the exact count.
	importedBy, err := db.GetImportedBy(ctx, unit.Path, unit.ModulePath, mainPageImportedByLimit)
	if err != nil {
		return "", err
	}
	if len(importedBy) < mainPageImportedByLimit {
		// Exact number is less than the limit, so just return that.
		return strconv.Itoa(len(importedBy)), nil
	}

	// Exact number is greater than the limit, so fetch an approximate value
	// from search_documents.num_imported_by. This number might be different
	// than the result of GetImportedBy because alternative modules and internal
	// packages are excluded.
	var count int
	if experiment.IsActive(ctx, internal.ExperimentGetUnitWithOneQuery) {
		count = unit.NumImportedBy
	} else {
		count, err = db.GetImportedByCount(ctx, unit.Path, unit.ModulePath)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Errorf(ctx, "missing search_documents row for path %s, module path %s", unit.Path, unit.ModulePath)
				return "", nil
			}
			return "", err
		}
		if count < mainPageImportedByLimit {
			count = mainPageImportedByLimit
		}
	}
	// Treat the result as approximate.
	return fmt.Sprintf("%d+", approximateLowerBound(count)), nil
}

// approximateLowerBound rounds n down to a multiple of a power of 10.
// See the test for examples.
func approximateLowerBound(n int) int {
	if n == 0 {
		return 0
	}
	f := float64(n)
	powerOf10 := math.Pow(10, math.Floor(math.Log10(f)))
	return int(powerOf10 * math.Floor(f/powerOf10))
}
