// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

// UnitPage contains data needed to render the unit template.
type UnitPage struct {
	basePage
	// Unit is the unit for this page.
	Unit *internal.UnitMeta

	// Breadcrumb contains data used to render breadcrumb UI elements.
	Breadcrumb breadcrumb

	// Title is the title of the page.
	Title string

	// URLPath is the path suitable for links on the page.
	// See the unitURLPath for details.
	URLPath string

	// CanonicalURLPath is a permanent representation of the URL path for a
	// unit.
	// It uses the resolved module path and version.
	// For example, if the latest version of /my.module/pkg is version v1.5.2,
	// the canonical URL path for that unit would be /my.module@v1.5.2/pkg
	CanonicalURLPath string

	// The version string formatted for display.
	DisplayVersion string

	// LinkVersion is version string suitable for links used to compute
	// latest badges.
	LinkVersion string

	// LatestURL is a url pointing to the latest version of a unit.
	LatestURL string

	// PageType is the type of page (pkg, cmd, dir, std, or mod).
	PageType string

	// PageLabels are the labels that will be displayed
	// for a given page.
	PageLabels []string

	// CanShowDetails indicates whether details can be shown or must be
	// hidden due to issues like license restrictions.
	CanShowDetails bool

	// Settings contains settings for the selected tab.
	SelectedTab TabSettings

	// Details contains data specific to the type of page being rendered.
	Details interface{}
}

// serveUnitPage serves a unit page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveUnitPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
	ds internal.DataSource, info *urlPathInfo) (err error) {
	defer derrors.Wrap(&err, "serveUnitPage(ctx, w, r, ds, %v)", info)

	tab := r.FormValue("tab")
	if tab == "" {
		// Default to details tab when there is no tab param.
		tab = tabMain
	}
	// Redirect to clean URL path when tab param is invalid.
	if _, ok := unitTabLookup[tab]; !ok {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}

	um, err := ds.GetUnitMeta(ctx, info.fullPath, info.modulePath, info.requestedVersion)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return err
		}
		return s.servePathNotFoundPage(w, r, ds, info.fullPath, info.modulePath, info.requestedVersion)
	}

	recordVersionTypeMetric(ctx, info.requestedVersion)
	if _, ok := internal.DefaultBranches[info.requestedVersion]; ok {
		// Since path@master is a moving target, we don't want it to be stale.
		// As a result, we enqueue every request of path@master to the frontend
		// task queue, which will initiate a fetch request depending on the
		// last time we tried to fetch this module version.
		//
		// Use a separate context here to prevent the context from being canceled
		// elsewhere before a task is enqueued.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			log.Infof(ctx, "serveUnitPage: Scheduling %q@%q to be fetched", info.modulePath, info.requestedVersion)
			if _, err := s.queue.ScheduleFetch(ctx, info.modulePath, info.requestedVersion, "", false); err != nil {
				log.Errorf(ctx, "serveUnitPage(%q): %v", r.URL.Path, err)
			}
		}()
	}

	if !isValidTabForUnit(tab, um) {
		// Redirect to clean URL path when tab param is invalid for the unit
		// type.
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}
	tabSettings := unitTabLookup[tab]
	title := pageTitle(um)
	basePage := s.newBasePage(r, title)
	basePage.AllowWideContent = true
	page := UnitPage{
		basePage:         basePage,
		Unit:             um,
		Breadcrumb:       displayBreadcrumb(um, info.requestedVersion),
		Title:            title,
		SelectedTab:      tabSettings,
		URLPath:          constructUnitURL(um.Path, um.ModulePath, info.requestedVersion),
		CanonicalURLPath: canonicalURLPath(um),
		DisplayVersion:   displayVersion(um.Version, um.ModulePath),
		LinkVersion:      linkVersion(um.Version, um.ModulePath),
		LatestURL:        constructUnitURL(um.Path, um.ModulePath, internal.LatestVersion),
		PageLabels:       pageLabels(um),
		PageType:         pageType(um),
	}
	d, err := fetchDetailsForUnit(ctx, r, tab, ds, um)
	if err != nil {
		return err
	}
	page.Details = d
	main, ok := d.(*MainDetails)
	if ok {
		page.MetaDescription = metaDescription(main.ImportedByCount)
	}
	s.servePage(ctx, w, tabSettings.TemplateName, page)
	return nil
}

// metaDescription uses a safehtml escape hatch to build HTML used
// to render the <meta name="Description"> for unit pages as a
// workaround for https://github.com/google/safehtml/issues/6.
func metaDescription(synopsis string) safehtml.HTML {
	if synopsis == "" {
		return safehtml.HTML{}
	}
	return safehtml.HTMLConcat(
		uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(`<meta name="Description" content="`),
		safehtml.HTMLEscaped(synopsis),
		uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(`">`),
	)
}

// isValidTabForUnit reports whether the tab is valid for the given unit.
// It is assumed that tab is a key in unitTabLookup.
func isValidTabForUnit(tab string, um *internal.UnitMeta) bool {
	if tab == tabLicenses && !um.IsRedistributable {
		return false
	}
	if !um.IsPackage() && (tab == tabImports || tab == tabImportedBy) {
		return false
	}
	return true
}

// constructUnitURL returns a URL path that refers to the given unit at the requested
// version. If requestedVersion is "latest", then the resulting path has no
// version; otherwise, it has requestedVersion.
func constructUnitURL(fullPath, modulePath, requestedVersion string) string {
	if requestedVersion == internal.LatestVersion {
		return "/" + fullPath
	}
	v := linkVersion(requestedVersion, modulePath)
	if fullPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", fullPath, v)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, v, strings.TrimPrefix(fullPath, modulePath+"/"))
}

// canonicalURLPath constructs a URL path to the unit that always includes the
// resolved version.
func canonicalURLPath(um *internal.UnitMeta) string {
	return constructUnitURL(um.Path, um.ModulePath, linkVersion(um.Version, um.ModulePath))
}
