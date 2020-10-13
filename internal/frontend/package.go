// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

// handlePackageDetailsRedirect redirects all redirects to "/pkg" to "/".
func (s *Server) handlePackageDetailsRedirect(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/pkg")
	http.Redirect(w, r, urlPath, http.StatusMovedPermanently)
}

// stdlibPathForShortcut returns a path in the stdlib that shortcut should redirect to,
// or the empty string if there is no such path.
func stdlibPathForShortcut(ctx context.Context, ds internal.DataSource, shortcut string) (path string, err error) {
	defer derrors.Wrap(&err, "stdlibPathForShortcut(ctx, %q)", shortcut)
	if !stdlib.Contains(shortcut) {
		return "", nil
	}
	db, ok := ds.(*postgres.DB)
	if !ok {
		return "", proxydatasourceNotSupportedErr()
	}
	matches, err := db.GetStdlibPathsWithSuffix(ctx, shortcut)
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	// No matches, or ambiguous.
	return "", nil
}

// servePackagePage serves a package details page.
func (s *Server) servePackagePage(ctx context.Context,
	w http.ResponseWriter, r *http.Request, ds internal.DataSource, um *internal.UnitMeta, requestedVersion string) error {
	mi := &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
	}
	pkgHeader, err := createPackage(&internal.PackageMeta{
		Path:              um.Path,
		Licenses:          um.Licenses,
		IsRedistributable: um.IsRedistributable,
		Name:              um.Name,
	}, mi, requestedVersion == internal.LatestVersion)
	if err != nil {
		return fmt.Errorf("creating package header for %s@%s: %v", um.Path, um.Version, err)
	}

	settings, err := packageSettings(r.FormValue("tab"))
	if err != nil {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}
	canShowDetails := um.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(r, settings.Name, ds, um)
		if err != nil {
			return fmt.Errorf("fetching page for %q: %v", settings.Name, err)
		}
	}
	var (
		pageType = legacyPageTypePackage
		pageName = um.Name
	)
	if pageName == "main" {
		pageName = effectiveName(um.Path, um.Name)
		pageType = legacyPageTypeCommand
	}
	page := &DetailsPage{
		basePage: s.newBasePage(r, packageHTMLTitle(um.Path, um.Name)),
		Name:     pageName,
		Settings: *settings,
		Header:   pkgHeader,
		Breadcrumb: breadcrumbPath(pkgHeader.Path, pkgHeader.Module.ModulePath,
			pkgHeader.Module.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           packageTabSettings,
		PageType:       pageType,
		CanonicalURLPath: constructPackageURL(
			pkgHeader.Path,
			pkgHeader.Module.ModulePath,
			pkgHeader.Module.LinkVersion),
	}
	page.basePage.AllowWideContent = settings.Name == legacyTabDoc
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}

// packageSettings returns the TabSettings corresponding to tab.
// If tab is not a valid tab from packageTabLookup or tab=doc, an error will be
// returned and the user will be redirected to /<path> outside of this
// function. If tab is the empty string, the user will be shown the
// documentation page.
func packageSettings(tab string) (*TabSettings, error) {
	if tab == legacyTabDoc {
		// Redirect "/<path>?tab=doc" to "/<path>".
		return nil, derrors.NotFound
	}
	if tab == "" {
		tab = legacyTabDoc
	}
	settings, ok := packageTabLookup[tab]
	if !ok {
		return nil, derrors.NotFound
	}
	return &settings, nil
}
