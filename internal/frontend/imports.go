// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/text/message"
)

// ImportsDetails contains information for a package's imports.
type ImportsDetails struct {
	ModulePath string

	// ExternalImports is the collection of package imports that are not in
	// the Go standard library and are not part of the same module
	ExternalImports []string

	// InternalImports is an array of packages representing the package's
	// imports that are part of the same module.
	InternalImports []string

	// StdLib is an array of packages representing the package's imports
	// that are in the Go standard library.
	StdLib []string
}

// fetchImportsDetails fetches imports for the package version specified by
// pkgPath, modulePath and version from the database and returns a ImportsDetails.
func fetchImportsDetails(ctx context.Context, ds internal.DataSource, pkgPath, modulePath, resolvedVersion string) (_ *ImportsDetails, err error) {
	u, err := ds.GetUnit(ctx, &internal.UnitMeta{
		Path: pkgPath,
		ModuleInfo: internal.ModuleInfo{
			ModulePath: modulePath,
			Version:    resolvedVersion,
		},
	}, internal.WithImports, internal.BuildContext{})
	if err != nil {
		return nil, err
	}

	var externalImports, moduleImports, std []string
	for _, p := range u.Imports {
		if stdlib.Contains(p) {
			std = append(std, p)
		} else if strings.HasPrefix(p+"/", modulePath+"/") {
			moduleImports = append(moduleImports, p)
		} else {
			externalImports = append(externalImports, p)
		}
	}

	return &ImportsDetails{
		ModulePath:      modulePath,
		ExternalImports: externalImports,
		InternalImports: moduleImports,
		StdLib:          std,
	}, nil
}

// ImportedByDetails contains information for the collection of packages that
// import a given package.
type ImportedByDetails struct {
	// ModulePath is the module path for the package referenced on this page.
	ModulePath string

	// ImportedBy is the collection of packages that import the
	// given package and are not part of the same module.
	// They are organized into a tree of sections by prefix.
	ImportedBy []*Section

	// NumImportedByDisplay is the display text at the top of the imported by
	// tab section, which shows the imported by count and package limit.
	NumImportedByDisplay string

	// Total is the total number of importers.
	Total int
}

var (
	// importedByLimit is the maximum number of importers displayed on the imported
	// by page.
	// Variable for testing.
	importedByLimit = 20001
)

// fetchImportedByDetails fetches importers for the package version specified by
// path and version from the database and returns a ImportedByDetails.
func fetchImportedByDetails(ctx context.Context, ds internal.DataSource, pkgPath, modulePath string) (*ImportedByDetails, error) {
	db, ok := ds.(internal.PostgresDB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return nil, datasourceNotSupportedErr()
	}

	importedBy, err := db.GetImportedBy(ctx, pkgPath, modulePath, importedByLimit)
	if err != nil {
		return nil, err
	}
	numImportedBy := len(importedBy)
	numImportedBySearch, err := db.GetImportedByCount(ctx, pkgPath, modulePath)
	if err != nil {
		return nil, err
	}
	if numImportedBy < importedByLimit && numImportedBySearch > numImportedBy {
		// Unless we hit the limit, numImportedBySearch should never be greater
		// than numImportedBy. If that happens, log an error so that we can
		// debug, but continue with generating the page fo the user.
		log.Errorf(ctx, "pkg %q, module %q: search_documents.num_imported_by %d > numImportedBy %d from imports unique, which shouldn't happen",
			pkgPath, modulePath, numImportedBySearch, numImportedBy)
	}

	if numImportedBy >= importedByLimit {
		importedBy = importedBy[:importedByLimit-1]
	}
	sections := Sections(importedBy, nextPrefixAccount)

	// Display the number of importers, taking into account the number we
	// actually retrieved, the limit on that number, and the imported-by count
	// in the search_documents table.
	pr := message.NewPrinter(middleware.LanguageTag(ctx))
	var (
		display string
		pkgword = "package"
	)
	if numImportedBy > 1 {
		pkgword = "packages"
	}
	switch {
	// If there are more importers than the limit, and the search number is
	// greater, use the search number and indicate that we're displaying fewer.
	case numImportedBy >= importedByLimit && numImportedBySearch > numImportedBy:
		display = pr.Sprintf("%d (displaying %d %s)", numImportedBySearch, importedByLimit-1, pkgword)
	// If we've exceeded the limit but the search number is smaller, we don't
	// know the true number, so say so.
	case numImportedBy >= importedByLimit:
		display = pr.Sprintf("%d (displaying more than %d %s, including internal and invalid packages)", numImportedBySearch, importedByLimit-1, pkgword)
	// If we haven't exceeded the limit and we have more than the search number,
	// then display both numbers so users coming from the search page won't see
	// a mismatch.
	case numImportedBy > numImportedBySearch:
		display = pr.Sprintf("%d (displaying %d %s, including internal and invalid packages)", numImportedBySearch, numImportedBy, pkgword)
	// Otherwise, we have all the packages, and the search number is either
	// wrong (perhaps it hasn't been recomputed yet) or it is the same as the
	// retrieved number. In that case, just display the retrieved number.
	default:
		display = pr.Sprint(numImportedBy)
	}
	return &ImportedByDetails{
		ModulePath:           modulePath,
		ImportedBy:           sections,
		NumImportedByDisplay: display,
		Total:                numImportedBy,
	}, nil
}
