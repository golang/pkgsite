// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

const (
	tabMain       = ""
	tabVersions   = "versions"
	tabImports    = "imports"
	tabImportedBy = "importedby"
	tabLicenses   = "licenses"
)

var (
	unitTabs = []TabSettings{
		{
			Name:         tabMain,
			TemplateName: "unit_details.tmpl",
		},
		{
			Name:         tabVersions,
			TemplateName: "unit_versions.tmpl",
		},
		{
			Name:         tabImports,
			TemplateName: "unit_imports.tmpl",
		},
		{
			Name:         tabImportedBy,
			TemplateName: "unit_importedby.tmpl",
		},
		{
			Name:         tabLicenses,
			TemplateName: "unit_licenses.tmpl",
		},
	}
	unitTabLookup = make(map[string]TabSettings, len(unitTabs))
)

func init() {
	for _, t := range unitTabs {
		unitTabLookup[t.Name] = t
	}
}

// fetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForUnit(ctx context.Context, r *http.Request, tab string, ds internal.DataSource, um *internal.UnitMeta) (_ interface{}, err error) {
	defer derrors.Wrap(&err, "fetchDetailsForUnit(r, %q, ds, um=%q,%q,%q)", tab, um.Path, um.ModulePath, um.Version)
	switch tab {
	case tabMain:
		_, expandReadme := r.URL.Query()["readme"]
		return fetchMainDetails(ctx, ds, um, expandReadme)
	case tabVersions:
		return fetchVersionsDetails(ctx, ds, um.Path, um.ModulePath)
	case tabImports:
		return fetchImportsDetails(ctx, ds, um.Path, um.ModulePath, um.Version)
	case tabImportedBy:
		return fetchImportedByDetails(ctx, ds, um.Path, um.ModulePath)
	case tabLicenses:
		return fetchLicensesDetails(ctx, ds, um)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}
