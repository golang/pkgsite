// Copyright 2019 The Go Authors. All rights reserved.
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

	"github.com/google/safehtml/template"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
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

const (
	legacyPageTypeModule    = "mod"
	legacyPageTypeDirectory = "dir"
	legacyPageTypePackage   = "pkg"
	legacyPageTypeCommand   = "cmd"
	legacyPageTypeModuleStd = "std"
)

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
			if _, err := s.queue.ScheduleFetch(ctx, urlInfo.modulePath, internal.MasterVersion, "", s.taskIDChangeInterval); err != nil {
				log.Errorf(ctx, "serveDetails(%q): %v", r.URL.Path, err)
			}
		}()
	}
	if experiment.IsActive(ctx, internal.ExperimentUnitPage) {
		return s.serveUnitPage(ctx, w, r, ds, um, urlInfo.requestedVersion)
	}
	return s.serveDetailsPage(w, r, ds, um, urlInfo)
}

// serveDetailsPage serves a details page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveDetailsPage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, um *internal.UnitMeta, info *urlPathInfo) (err error) {
	defer derrors.Wrap(&err, "serveDetailsPage(w, r, %v)", info)
	ctx := r.Context()

	if info.isModule {
		return s.serveModulePage(ctx, w, r, ds, um, info.requestedVersion)
	}
	if um.IsPackage() {
		return s.servePackagePage(ctx, w, r, ds, um, info.requestedVersion)
	}
	return s.serveDirectoryPage(ctx, w, r, ds, um, info.requestedVersion)
}

type urlPathInfo struct {
	// fullPath is the full import path corresponding to the requested
	// package/module/directory page.
	fullPath string
	// isModule indicates whether the /mod page should be shown.
	isModule bool
	// modulePath is the path of the module corresponding to the fullPath and
	// resolvedVersion. If unknown, it is set to internal.UnknownModulePath.
	modulePath string
	// requestedVersion is the version requested by the user, which will be one
	// of the following: "latest", "master", a Go version tag, or a semantic
	// version.
	requestedVersion string
	// resolvedVersion is the semantic version stored in the database.
	resolvedVersion string
}

func extractURLPathInfo(urlPath string) (_ *urlPathInfo, err error) {
	defer derrors.Wrap(&err, "extractURLPathInfo(%q)", urlPath)

	info := &urlPathInfo{}
	if strings.HasPrefix(urlPath, "/mod/") {
		urlPath = strings.TrimPrefix(urlPath, "/mod")
		info.isModule = true
	}
	// Parse the fullPath, modulePath and requestedVersion, based on whether
	// the path is in the stdlib. If unable to parse these elements, return
	// http.StatusBadRequest.
	if parts := strings.SplitN(strings.TrimPrefix(urlPath, "/"), "@", 2); stdlib.Contains(parts[0]) {
		info.fullPath, info.requestedVersion, err = parseStdLibURLPath(urlPath)
		info.modulePath = stdlib.ModulePath
		if info.fullPath == stdlib.ModulePath {
			info.isModule = true
		}
	} else {
		info.fullPath, info.modulePath, info.requestedVersion, err = parseDetailsURLPath(urlPath)
	}
	if err != nil {
		return nil, err
	}
	return info, nil
}

// parseDetailsURLPath parses a URL path that refers (or may refer) to something
// in the Go ecosystem.
//
// After trimming leading and trailing slashes, the path is expected to have one
// of three forms, and we divide it into three parts: a full path, a module
// path, and a version.
//
// 1. The path has no '@', like github.com/hashicorp/vault/api.
//    This is the full path. The module path is unknown. So is the version, so we
//    treat it as the latest version for whatever the path denotes.
//
// 2. The path has "@version" at the end, like github.com/hashicorp/vault/api@v1.2.3.
//    We split this at the '@' into a full path (github.com/hashicorp/vault/api)
//    and version (v1.2.3); the module path is still unknown.
//
// 3. The path has "@version" in the middle, like github.com/hashicorp/vault@v1.2.3/api.
//    (We call this the "canonical" form of a path.)
//    We remove the version to get the full path, which is again
//    github.com/hashicorp/vault/api. The version is v1.2.3, and the module path is
//    the part before the '@', github.com/hashicorp/vault.
//
// In one case, we do a little more than parse the urlPath into parts: if the full path
// could be a part of the standard library (because it has no '.'), we assume it
// is and set the modulePath to indicate the standard library.
func parseDetailsURLPath(urlPath string) (fullPath, modulePath, requestedVersion string, err error) {
	defer derrors.Wrap(&err, "parseDetailsURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<module-path>[/<suffix>]
	// or
	//   /<module-path>, @<version>/<suffix>
	// or
	//  /<module-path>/<suffix>, @<version>
	parts := strings.SplitN(urlPath, "@", 2)
	basePath := strings.TrimSuffix(strings.TrimPrefix(parts[0], "/"), "/")
	if len(parts) == 1 { // no '@'
		modulePath = internal.UnknownModulePath
		requestedVersion = internal.LatestVersion
		fullPath = basePath
	} else {
		// Parse the version and suffix from parts[1], the string after the '@'.
		endParts := strings.Split(parts[1], "/")
		suffix := strings.Join(endParts[1:], "/")
		// The first path component after the '@' is the version.
		requestedVersion = endParts[0]
		// You cannot explicitly write "latest" for the version.
		if requestedVersion == internal.LatestVersion {
			return "", "", "", fmt.Errorf("invalid version: %q", requestedVersion)
		}
		if suffix == "" {
			// "@version" occurred at the end of the path; we don't know the module path.
			modulePath = internal.UnknownModulePath
			fullPath = basePath
		} else {
			// "@version" occurred in the middle of the path; the part before it
			// is the module path.
			modulePath = basePath
			fullPath = basePath + "/" + suffix
		}
	}
	// The full path must be a valid import path (that is, package path), even if it denotes
	// a module, directory or collection.
	if err := module.CheckImportPath(fullPath); err != nil {
		return "", "", "", fmt.Errorf("malformed path %q: %v", fullPath, err)
	}

	// If the full path is (or could be) in the standard library, change the
	// module path to say so. But in that case, disallow versions in the middle,
	// like "net@go1.14/http". That says that the module is "net", and it isn't.
	if stdlib.Contains(fullPath) {
		if modulePath != internal.UnknownModulePath {
			return "", "", "", fmt.Errorf("non-final version in standard library path %q", urlPath)
		}
		modulePath = stdlib.ModulePath
	}
	return fullPath, modulePath, requestedVersion, nil
}

// validatePathAndVersion verifies that the requested path and version are
// acceptable. The given path may be a module or package path.
func validatePathAndVersion(ctx context.Context, ds internal.DataSource, fullPath, requestedVersion string) error {
	if !isSupportedVersion(fullPath, requestedVersion) {
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
	db, ok := ds.(*postgres.DB)
	if !ok {
		return nil
	}
	excluded, err := db.IsExcluded(ctx, fullPath)
	if err != nil {
		return err
	}
	if excluded {
		// Return NotFound; don't let the user know that the package was excluded.
		return &serverError{status: http.StatusNotFound}
	}
	return nil
}

// isSupportedVersion reports whether the version is supported by the frontend.
func isSupportedVersion(fullPath, requestedVersion string) bool {
	if stdlib.Contains(fullPath) && requestedVersion == internal.MasterVersion {
		return false
	}
	if requestedVersion == internal.LatestVersion || semver.IsValid(requestedVersion) {
		return true
	}
	return requestedVersion == internal.MasterVersion
}

// pathNotFoundError returns a page with an option on how to
// add a package or module to the site.
func pathNotFoundError(fullPath, requestedVersion string) error {
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

func proxydatasourceNotSupportedErr() error {
	return &serverError{
		status: http.StatusFailedDependency,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(
				`<h3 class="Error-message">This page is not supported by the proxydatasource.</h3>`),
		},
	}
}

func parseStdLibURLPath(urlPath string) (path, requestedVersion string, err error) {
	defer derrors.Wrap(&err, "parseStdLibURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<path>@<tag> or /<path>
	parts := strings.SplitN(urlPath, "@", 2)
	path = strings.TrimSuffix(strings.TrimPrefix(parts[0], "/"), "/")
	if err := module.CheckImportPath(path); err != nil {
		return "", "", err
	}

	if len(parts) == 1 {
		return path, internal.LatestVersion, nil
	}
	requestedVersion = stdlib.VersionForTag(strings.TrimSuffix(parts[1], "/"))
	if requestedVersion == "" {
		return "", "", fmt.Errorf("invalid Go tag for url: %q", urlPath)
	}
	return path, requestedVersion, nil
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
		if fr.status == statusNotFoundInVersionMap || fr.status == http.StatusInternalServerError {
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

func setExperimentsFromQueryParam(ctx context.Context, r *http.Request) context.Context {
	if err := r.ParseForm(); err != nil {
		log.Errorf(ctx, "ParseForm: %v", err)
		return ctx
	}
	return newContextFromExps(ctx, r.Form["exp"])
}

// newContextFromExps adds and removes experiments from the context's experiment
// set, creates a new set with the changes, and returns a context with the new
// set. Each string in expMods can be either an experiment name, which means
// that the experiment should be added, or "!" followed by an experiment name,
// meaning that it should be removed.
func newContextFromExps(ctx context.Context, expMods []string) context.Context {
	var (
		exps   []string
		remove = map[string]bool{}
	)
	set := experiment.FromContext(ctx)
	for _, exp := range expMods {
		if strings.HasPrefix(exp, "!") {
			exp = exp[1:]
			if set.IsActive(exp) {
				remove[exp] = true
			}
		} else if !set.IsActive(exp) {
			exps = append(exps, exp)
		}
	}
	if len(exps) == 0 && len(remove) == 0 {
		return ctx
	}
	for _, a := range set.Active() {
		if !remove[a] {
			exps = append(exps, a)
		}
	}
	return experiment.NewContext(ctx, exps...)
}
