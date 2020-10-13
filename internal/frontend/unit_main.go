// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strconv"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
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

func fetchMainDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) (_ *MainDetails, err error) {
	unit, err := ds.GetUnit(ctx, um, internal.AllFields)
	if err != nil {
		return nil, err
	}

	// importedByCount is not supported when using a datasource proxy.
	importedByCount := "0"
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
		docHTML := getHTML(ctx, unit)
		// TODO: Deprecate godoc.Parse. The sidenav and body can
		// either be rendered using separate functions, or all this content can
		// be passed to the template via the UnitPage struct.
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

		files, err = sourceFiles(unit)
		if err != nil {
			return nil, err
		}
	}
	return &MainDetails{
		NestedModules:   nestedModules,
		Subdirectories:  subdirectories,
		Licenses:        transformLicenseMetadata(um.Licenses),
		CommitTime:      elapsedTime(um.CommitTime),
		Readme:          readme,
		DocOutline:      docOutline,
		DocBody:         docBody,
		SourceFiles:     files,
		MobileOutline:   mobileOutline,
		NumImports:      len(unit.Imports),
		ImportedByCount: importedByCount,
		IsPackage:       unit.IsPackage(),
	}, nil
}
