// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// handleModuleDetails handles requests for non-stdlib module details pages. It
// expects paths of the form "/mod/<module-path>[@<version>?tab=<tab>]".
// stdlib module pages are handled at "/std".
func (s *Server) handleModuleDetails(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/mod/std" {
		http.Redirect(w, r, "/std", http.StatusMovedPermanently)
		return
	}
	urlPath := strings.TrimPrefix(r.URL.Path, "/mod")
	path, _, version, err := parseDetailsURLPath(urlPath)
	if err != nil {
		log.Infof(r.Context(), "handleModuleDetails: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	s.serveModulePage(w, r, path, version)
}

// serveModulePage serves details pages for the module specified by modulePath
// and version.
func (s *Server) serveModulePage(w http.ResponseWriter, r *http.Request, modulePath, version string) {
	ctx := r.Context()
	if code, epage := checkPathAndVersion(ctx, s.ds, modulePath, version); code != http.StatusOK {
		s.serveErrorPage(w, r, code, epage)
		return
	}
	// This function handles top level behavior related to the existence of the
	// requested modulePath@version:
	// TODO: fix
	//   1. If the module version exists, serve it.
	//   2. else if we got any unexpected error, serve a server error
	//   3. else if the error is NotFound, serve the directory page
	//   3. else, we didn't find the module so there are two cases:
	//     a. We don't know anything about this module: just serve a 404
	//     b. We have valid versions for this module path, but `version` isn't
	//        one of them. Serve a 404 but recommend the other versions.
	vi, err := s.ds.GetVersionInfo(ctx, modulePath, version)
	if err == nil {
		s.serveModulePageWithModule(ctx, w, r, vi, version)
		return
	}
	if !errors.Is(err, derrors.NotFound) {
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	if version != internal.LatestVersion {
		if _, err := s.ds.GetVersionInfo(ctx, modulePath, internal.LatestVersion); err != nil {
			log.Errorf(ctx, "error checking for latest module: %v", err)
		} else {
			epage := &errorPage{
				Message: fmt.Sprintf("Module %s@%s is not available.", modulePath, displayVersion(version, modulePath)),
				SecondaryMessage: template.HTML(
					fmt.Sprintf(`There are other versions of this module that are! To view them, `+
						`<a href="/mod/%s?tab=versions">click here</a>.</p>`,
						modulePath)),
			}
			s.serveErrorPage(w, r, http.StatusNotFound, epage)
			return
		}
	}
	s.serveErrorPage(w, r, http.StatusNotFound, nil)
}

func (s *Server) serveModulePageWithModule(ctx context.Context, w http.ResponseWriter, r *http.Request, vi *internal.VersionInfo, requestedVersion string) {
	licenses, err := s.ds.GetModuleLicenses(ctx, vi.ModulePath, vi.Version)
	if err != nil {
		log.Errorf(ctx, "error getting module licenses: %v", err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	modHeader := createModule(vi, licensesToMetadatas(licenses), requestedVersion == internal.LatestVersion)
	tab := r.FormValue("tab")
	settings, ok := moduleTabLookup[tab]
	if !ok {
		tab = "overview"
		settings = moduleTabLookup["overview"]
	}
	canShowDetails := modHeader.IsRedistributable || settings.AlwaysShowDetails
	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForModule(ctx, r, tab, s.ds, vi, licenses)
		if err != nil {
			log.Errorf(ctx, "error fetching page for %q: %v", tab, err)
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
	}
	page := &DetailsPage{
		basePage:       newBasePage(r, moduleHTMLTitle(vi.ModulePath)),
		Title:          moduleTitle(vi.ModulePath),
		Settings:       settings,
		Header:         modHeader,
		BreadcrumbPath: breadcrumbPath(modHeader.ModulePath, modHeader.ModulePath, modHeader.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		PageType:       "mod",
	}
	s.servePage(ctx, w, settings.TemplateName, page)
}
