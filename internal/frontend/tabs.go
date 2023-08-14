// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend/versions"
	"golang.org/x/pkgsite/internal/vuln"
)

// TabSettings defines tab-specific metadata.
type TabSettings struct {
	// Name is the tab name used in the URL.
	Name string

	// DisplayName is the formatted tab name.
	DisplayName string

	// AlwaysShowDetails defines whether the tab content can be shown even if the
	// package is not determined to be redistributable.
	AlwaysShowDetails bool

	// TemplateName is the name of the template used to render the
	// corresponding tab, as defined in Server.templates.
	TemplateName string

	// Disabled indicates whether a tab should be displayed as disabled.
	Disabled bool
}

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
			TemplateName: "unit/main",
		},
		{
			Name:         tabVersions,
			TemplateName: "unit/versions",
		},
		{
			Name:         tabImports,
			TemplateName: "unit/imports",
		},
		{
			Name:         tabImportedBy,
			TemplateName: "unit/importedby",
		},
		{
			Name:         tabLicenses,
			TemplateName: "unit/licenses",
		},
	}
	unitTabLookup = make(map[string]TabSettings, len(unitTabs))
)

func init() {
	for _, t := range unitTabs {
		unitTabLookup[t.Name] = t
	}
}

// fetchDetailsForUnit returns tab details by delegating to the correct detail
// handler.
func fetchDetailsForUnit(ctx context.Context, r *http.Request, tab string, ds internal.DataSource, um *internal.UnitMeta,
	requestedVersion string, bc internal.BuildContext,
	vc *vuln.Client) (_ any, err error) {
	defer derrors.Wrap(&err, "fetchDetailsForUnit(r, %q, ds, um=%q,%q,%q)", tab, um.Path, um.ModulePath, um.Version)
	switch tab {
	case tabMain:
		_, expandReadme := r.URL.Query()["readme"]
		return fetchMainDetails(ctx, ds, um, requestedVersion, expandReadme, bc)
	case tabVersions:
		return versions.FetchVersionsDetails(ctx, ds, um, vc)
	case tabImports:
		return fetchImportsDetails(ctx, ds, um.Path, um.ModulePath, um.Version)
	case tabImportedBy:
		return fetchImportedByDetails(ctx, ds, um.Path, um.ModulePath)
	case tabLicenses:
		return fetchLicensesDetails(ctx, ds, um)
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}
