// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
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
	var dsImports []string
	if isActiveUseUnits(ctx) {
		u, err := ds.GetUnit(ctx, &internal.UnitMeta{
			Path:       pkgPath,
			ModulePath: modulePath,
			Version:    resolvedVersion,
		}, internal.WithImports)
		if err != nil {
			return nil, err
		}
		dsImports = u.Imports
	} else {
		dsImports, err = ds.LegacyGetImports(ctx, pkgPath, modulePath, resolvedVersion)
		if err != nil {
			return nil, err
		}
	}

	var externalImports, moduleImports, std []string
	for _, p := range dsImports {
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
	ModulePath string

	// ImportedBy is the collection of packages that import the
	// given package and are not part of the same module.
	// They are organized into a tree of sections by prefix.
	ImportedBy []*Section

	Total        int  // number of packages in ImportedBy
	TotalIsExact bool // if false, then there may be more than Total
}

const importedByLimit = 20001

// etchImportedByDetails fetches importers for the package version specified by
// path and version from the database and returns a ImportedByDetails.
func fetchImportedByDetails(ctx context.Context, ds internal.DataSource, pkgPath, modulePath string) (*ImportedByDetails, error) {
	db, ok := ds.(*postgres.DB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return nil, proxydatasourceNotSupportedErr()
	}

	importedBy, err := db.GetImportedBy(ctx, pkgPath, modulePath, importedByLimit)
	if err != nil {
		return nil, err
	}
	// If we reached the query limit, then we don't know the total.
	// Say so, and show one less than the limit.
	// For example, if the limit is 101 and we get 101 results, then we'll
	// say there are more than 100, and show the first 100.
	totalIsExact := true
	if len(importedBy) == importedByLimit {
		importedBy = importedBy[:len(importedBy)-1]
		totalIsExact = false
	}
	sections := Sections(importedBy, nextPrefixAccount)
	return &ImportedByDetails{
		ModulePath:   modulePath,
		ImportedBy:   sections,
		Total:        len(importedBy),
		TotalIsExact: totalIsExact,
	}, nil
}
