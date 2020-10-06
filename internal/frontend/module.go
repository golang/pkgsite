// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

func (s *Server) serveModulePage(ctx context.Context, w http.ResponseWriter, r *http.Request, ds internal.DataSource,
	um *internal.UnitMeta, requestedVersion string) error {
	mi := &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
	}
	modHeader := createModule(mi, um.Licenses, requestedVersion == internal.LatestVersion)
	tab := r.FormValue("tab")
	settings, ok := moduleTabLookup[tab]
	if !ok {
		tab = tabOverview
		settings = moduleTabLookup[tabOverview]
	}
	canShowDetails := modHeader.IsRedistributable || settings.AlwaysShowDetails
	var details interface{}
	if canShowDetails {
		log.Debugf(ctx, "fetchDetailsForModule being called with um.Path=%q, um.ModulePath=%q", um.Path, um.ModulePath)
		var err error
		details, err = fetchDetailsForModule(r, tab, ds, um)
		if err != nil {
			return fmt.Errorf("error fetching page for %q: %v", tab, err)
		}
	}
	pageType := pageTypeModule
	if um.ModulePath == stdlib.ModulePath {
		pageType = pageTypeModuleStd
	}

	page := &DetailsPage{
		basePage:       s.newBasePage(r, moduleHTMLTitle(um.ModulePath)),
		Name:           um.ModulePath,
		Settings:       settings,
		Header:         modHeader,
		Breadcrumb:     breadcrumbPath(modHeader.ModulePath, modHeader.ModulePath, modHeader.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		PageType:       pageType,
		CanonicalURLPath: constructModuleURL(
			um.ModulePath,
			linkVersion(um.Version, um.ModulePath),
		),
	}
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}
