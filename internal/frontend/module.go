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
	vdir *internal.VersionedDirectory, requestedVersion string) error {
	modHeader := createModule(&vdir.ModuleInfo, vdir.Licenses, requestedVersion == internal.LatestVersion)
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
		details, err = fetchDetailsForModule(r, tab, ds, vdir)
		if err != nil {
			return fmt.Errorf("error fetching page for %q: %v", tab, err)
		}
	}
	pageType := pageTypeModule
	if vdir.ModulePath == stdlib.ModulePath {
		pageType = pageTypeStdLib
	}

	page := &DetailsPage{
		basePage:       s.newBasePage(r, moduleHTMLTitle(vdir.ModulePath)),
		Name:           vdir.ModulePath,
		Settings:       settings,
		Header:         modHeader,
		Breadcrumb:     breadcrumbPath(modHeader.ModulePath, modHeader.ModulePath, modHeader.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		PageType:       pageType,
		CanonicalURLPath: constructModuleURL(
			vdir.ModulePath,
			linkVersion(vdir.Version, vdir.ModulePath),
		),
	}
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}
