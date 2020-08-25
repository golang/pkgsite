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
	w http.ResponseWriter, r *http.Request, ds internal.DataSource, dir *internal.Directory, requestedVersion string) error {
	pkgHeader, err := createPackage(&internal.PackageMeta{
		Path:              dir.Path,
		Licenses:          dir.Licenses,
		IsRedistributable: dir.IsRedistributable,
		Name:              dir.Package.Name,
	}, &dir.ModuleInfo, requestedVersion == internal.LatestVersion)
	if err != nil {
		return fmt.Errorf("creating package header for %s@%s: %v", dir.Path, dir.Version, err)
	}

	tab := r.FormValue("tab")
	settings, ok := packageTabLookup[tab]
	if !ok {
		var tab string
		if dir.IsRedistributable {
			tab = tabDoc
		} else {
			tab = tabOverview
		}
		http.Redirect(w, r, fmt.Sprintf(r.URL.Path+"?tab=%s", tab), http.StatusFound)
		return nil
	}
	canShowDetails := dir.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(r, tab, ds, dir)
		if err != nil {
			return fmt.Errorf("fetching page for %q: %v", tab, err)
		}
	}
	var (
		pageType = pageTypePackage
		pageName = dir.Package.Name
	)
	if pageName == "main" {
		pageName = effectiveName(dir.Path, dir.Package.Name)
		pageType = pageTypeCommand
	}
	page := &DetailsPage{
		basePage: s.newBasePage(r, packageHTMLTitle(dir.Path, dir.Package.Name)),
		Name:     pageName,
		Settings: settings,
		Header:   pkgHeader,
		Breadcrumb: breadcrumbPath(pkgHeader.Path, pkgHeader.Module.ModulePath,
			pkgHeader.Module.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           packageTabSettings,
		PageType:       pageType,
	}
	page.basePage.AllowWideContent = tab == tabDoc
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}
