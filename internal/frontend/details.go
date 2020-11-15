// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/safehtml/template"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DetailsPage contains data for a package of module details template.
type DetailsPage struct {
	basePage

	// Name is the name of the package or command name, or the full
	// directory or module path.
	Name string

	// PageType is the type of page (pkg, cmd, dir, etc.).
	PageType string

	// CanShowDetails indicates whether details can be shown or must be
	// hidden due to issues like license restrictions.
	CanShowDetails bool

	// Settings contains tab-specific metadata.
	Settings TabSettings

	// Details contains data specific to the type of page being rendered.
	Details interface{}

	// Header contains data to be rendered in the heading of all details pages.
	Header interface{}

	// Breadcrumb contains data used to render breadcrumb UI elements.
	Breadcrumb breadcrumb

	// Tabs contains data to render the varioius tabs on each details page.
	Tabs []TabSettings

	// CanonicalURLPath is the representation of the URL path for the details
	// page, after the requested version and module path have been resolved.
	// For example, if the latest version of /my.module/pkg is version v1.5.2,
	// the canonical url for that path would be /my.module@v1.5.2/pkg
	CanonicalURLPath string
}

var (
	keyVersionType     = tag.MustNewKey("frontend.version_type")
	versionTypeResults = stats.Int64(
		"go-discovery/frontend_version_type_count",
		"The version type of a request to package, module, or directory page.",
		stats.UnitDimensionless,
	)
	VersionTypeCount = &view.View{
		Name:        "go-discovery/frontend_version_type/result_count",
		Measure:     versionTypeResults,
		Aggregation: view.Count(),
		Description: "version type results, by latest, master, or semver",
		TagKeys:     []tag.Key{keyVersionType},
	}
)

// serveDetails handles requests for package/directory/module details pages. It
// expects paths of the form "[/mod]/<module-path>[@<version>?tab=<tab>]".
// stdlib module pages are handled at "/std", and requests to "/mod/std" will
// be redirected to that path.
func (s *Server) serveDetails(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer middleware.ElapsedStat(r.Context(), "serveDetails")()

	if r.Method != http.MethodGet {
		return &serverError{status: http.StatusMethodNotAllowed}
	}

	switch r.URL.Path {
	case "/":
		s.staticPageHandler("index.tmpl", "")(w, r)
		return nil
	case "/C":
		// Package "C" is a special case: redirect to the Go Blog article on cgo.
		// (This is what godoc.org does.)
		http.Redirect(w, r, "https://golang.org/doc/articles/c_go_cgo.html", http.StatusMovedPermanently)
		return nil
	case "/mod/std":
		// The stdlib module page is hosted at "/std".
		http.Redirect(w, r, "/std", http.StatusMovedPermanently)
		return nil
	}

	urlInfo, err := extractURLPathInfo(r.URL.Path)
	if err != nil {
		return &serverError{
			status: http.StatusBadRequest,
			err:    err,
		}
	}
	ctx := r.Context()
	// If page statistics are enabled, use the "exp" query param to adjust
	// the active experiments.
	if s.serveStats {
		ctx = setExperimentsFromQueryParam(ctx, r)
	}
	// Validate the fullPath and requestedVersion that were parsed.
	if err := validatePathAndVersion(ctx, ds, urlInfo.fullPath, urlInfo.requestedVersion); err != nil {
		return err
	}
	recordVersionTypeMetric(ctx, urlInfo.requestedVersion)

	urlInfo.resolvedVersion = urlInfo.requestedVersion
	um, err := ds.GetUnitMeta(ctx, urlInfo.fullPath, urlInfo.modulePath, urlInfo.requestedVersion)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return err
		}
		return s.servePathNotFoundPage(w, r, ds, urlInfo.fullPath, urlInfo.requestedVersion)
	}
	if urlInfo.isModule && um.ModulePath != urlInfo.fullPath {
		return s.servePathNotFoundPage(w, r, ds, urlInfo.fullPath, urlInfo.requestedVersion)
	}

	urlInfo.modulePath = um.ModulePath
	urlInfo.resolvedVersion = um.Version
	if urlInfo.requestedVersion == internal.MasterVersion {
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
			if _, err := s.queue.ScheduleFetch(ctx, urlInfo.modulePath, internal.MasterVersion, ""); err != nil {
				log.Errorf(ctx, "serveDetails(%q): %v", r.URL.Path, err)
			}
		}()
	}
	return s.serveUnitPage(ctx, w, r, ds, um, urlInfo.requestedVersion)
}

// pathNotFoundError returns a page with an option on how to
// add a package or module to the site.
func pathNotFoundError(fullPath, requestedVersion string) error {
	if !isSupportedVersion(fullPath, requestedVersion) {
		return invalidVersionError(fullPath, requestedVersion)
	}
	if stdlib.Contains(fullPath) {
		return &serverError{status: http.StatusNotFound}
	}
	path := fullPath
	if requestedVersion != internal.LatestVersion {
		path = fmt.Sprintf("%s@%s", fullPath, requestedVersion)
	}
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			templateName: "fetch.tmpl",
			MessageData:  path,
		},
	}
}

func invalidVersionError(fullPath, requestedVersion string) error {
	return &serverError{
		status: http.StatusBadRequest,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(`
					<h3 class="Error-message">{{.Version}} is not a valid semantic version.</h3>
					<p class="Error-message">
					  To search for packages like {{.Path}}, <a href="/search?q={{.Path}}">click here</a>.
					</p>`),
			MessageData: struct{ Path, Version string }{fullPath, requestedVersion},
		},
	}
}

func proxydatasourceNotSupportedErr() error {
	return &serverError{
		status: http.StatusFailedDependency,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(
				`<h3 class="Error-message">This page is not supported by the proxydatasource.</h3>`),
		},
	}
}

// errUnitNotFoundWithoutFetch returns a 404 with instructions to the user on
// how to manually fetch the package. No fetch button is provided. This is used
// for very large modules or modules that previously 500ed.
var errUnitNotFoundWithoutFetch = &serverError{
	status: http.StatusNotFound,
	epage: &errorPage{
		messageTemplate: template.MakeTrustedTemplate(`
					    <h3 class="Error-message">{{.StatusText}}</h3>
					    <p class="Error-message">Check that you entered the URL correctly or try fetching it following the
                        <a href="/about#adding-a-package">instructions here</a>.</p>`),
		MessageData: struct{ StatusText string }{http.StatusText(http.StatusNotFound)},
	},
}

func (s *Server) servePathNotFoundPage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, fullPath, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "servePathNotFoundPage(w, r, %q, %q)", fullPath, requestedVersion)

	ctx := r.Context()
	path, err := stdlibPathForShortcut(ctx, ds, fullPath)
	if err != nil {
		// Log the error, but prefer a "path not found" error for a
		// better user experience.
		log.Error(ctx, err)
	}
	if path != "" {
		http.Redirect(w, r, fmt.Sprintf("/%s", path), http.StatusFound)
		return
	}
	if stdlib.Contains(fullPath) {
		return &serverError{status: http.StatusNotFound}
	}
	db, ok := ds.(*postgres.DB)
	if !ok {
		return pathNotFoundError(fullPath, requestedVersion)
	}
	modulePaths, err := candidateModulePaths(fullPath)
	if err != nil {
		return err
	}
	results := s.checkPossibleModulePaths(ctx, db, fullPath, requestedVersion, modulePaths, false)
	for _, fr := range results {
		if fr.status == http.StatusInternalServerError {
			return errUnitNotFoundWithoutFetch
		}
		if fr.status == statusNotFoundInVersionMap {
			// If the result is statusNotFoundInVersionMap, it means that
			// we haven't attempted to fetch this path before. Return an
			// error page giving the user the option to fetch the path.
			return pathNotFoundError(fullPath, requestedVersion)
		}
	}
	status, responseText := fetchRequestStatusAndResponseText(results, fullPath, requestedVersion)
	return &serverError{
		status: status,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(`
					<h3 class="Error-message">{{.StatusText}}</h3>
					<p class="Error-message">{{.Response}}</p>`),
			MessageData: struct{ StatusText, Response string }{http.StatusText(status), responseText},
		},
	}
}

func recordVersionTypeMetric(ctx context.Context, requestedVersion string) {
	// Tag versions based on latest, master and semver.
	v := requestedVersion
	if semver.IsValid(v) {
		v = "semver"
	}
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyVersionType, v),
	}, versionTypeResults.M(1))
}
