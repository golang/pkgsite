// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

func (s *Server) serveModulePage(ctx context.Context, w http.ResponseWriter, r *http.Request, ds internal.DataSource,
	dmeta *internal.DirectoryMeta, requestedVersion string) error {
	modHeader := createModule(&dmeta.ModuleInfo, dmeta.Licenses, requestedVersion == internal.LatestVersion)
	tab := r.FormValue("tab")
	settings, ok := moduleTabLookup[tab]
	if !ok {
		tab = tabOverview
		settings = moduleTabLookup[tabOverview]
	}
	canShowDetails := modHeader.IsRedistributable || settings.AlwaysShowDetails
	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForModule(r, tab, ds, dmeta)
		if err != nil {
			return fmt.Errorf("error fetching page for %q: %v", tab, err)
		}
	}
	pageType := pageTypeModule
	if dmeta.ModulePath == stdlib.ModulePath {
		pageType = pageTypeStdLib
	}

	page := &DetailsPage{
		basePage:       s.newBasePage(r, moduleHTMLTitle(dmeta.ModulePath)),
		Name:           dmeta.ModulePath,
		Settings:       settings,
		Header:         modHeader,
		Breadcrumb:     breadcrumbPath(modHeader.ModulePath, modHeader.ModulePath, modHeader.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		PageType:       pageType,
		CanonicalURLPath: constructModuleURL(
			dmeta.ModulePath,
			linkVersion(dmeta.Version, dmeta.ModulePath),
		),
	}
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}
