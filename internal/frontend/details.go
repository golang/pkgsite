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

	"github.com/google/safehtml/template"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
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
}

const (
	pageTypeModule    = "mod"
	pageTypeDirectory = "dir"
	pageTypePackage   = "pkg"
	pageTypeCommand   = "cmd"
	pageTypeStdLib    = stdlib.ModulePath
)

// serveDetails handles requests for package/directory/module details pages. It
// expects paths of the form "[/mod]/<module-path>[@<version>?tab=<tab>]".
// stdlib module pages are handled at "/std", and requests to "/mod/std" will
// be redirected to that path.
func (s *Server) serveDetails(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
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
	// Validate the fullPath and requestedVersion that were parsed.
	if err := validatePathAndVersion(ctx, ds, urlInfo.fullPath, urlInfo.requestedVersion); err != nil {
		return err
	}

	urlInfo.resolvedVersion = urlInfo.requestedVersion
	if experiment.IsActive(ctx, internal.ExperimentUsePathInfo) {
		resolvedModulePath, resolvedVersion, _, err := ds.GetPathInfo(ctx, urlInfo.fullPath, urlInfo.modulePath, urlInfo.requestedVersion)
		if err != nil {
			if !errors.Is(err, derrors.NotFound) {
				return err
			}
			pathType := "package"
			if urlInfo.isModule {
				pathType = "module"
			}
			return s.servePathNotFoundPage(w, r, ds, urlInfo.fullPath, urlInfo.modulePath, urlInfo.requestedVersion, pathType)
		}
		urlInfo.modulePath = resolvedModulePath
		urlInfo.resolvedVersion = resolvedVersion

		if isActivePathAtMaster(ctx) && urlInfo.requestedVersion == internal.MasterVersion {
			// Since path@master is a moving target, we don't want it to be stale.
			// As a result, we enqueue every request of path@master to the frontend
			// task queue, which will initiate a fetch request depending on the
			// last time we tried to fetch this module version.
			go func() {
				if err := s.queue.ScheduleFetch(ctx, urlInfo.modulePath, internal.MasterVersion, "", s.taskIDChangeInterval); err != nil {
					log.Errorf(ctx, "serveDetails(%q): %v", r.URL.Path, err)
				}
			}()
		}
	}
	if isActiveUseDirectories(ctx) {
		return s.serveDetailsPage(w, r, ds, urlInfo)
	}
	return s.legacyServeDetailsPage(w, r, ds, urlInfo)
}

// serveDetailsPage serves a details page for a path using the paths,
// modules, documentation, readmes, licenses, and package_imports tables.
func (s *Server) serveDetailsPage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, info *urlPathInfo) (err error) {
	defer derrors.Wrap(&err, "serveDetailsPage(w, r, %v)", info)
	ctx := r.Context()
	vdir, err := ds.GetDirectory(ctx, info.fullPath, info.modulePath, info.resolvedVersion)
	if err != nil {
		return err
	}
	switch {
	case info.isModule:
		var readme *internal.Readme
		if vdir.Readme != nil {
			readme = &internal.Readme{Filepath: vdir.Readme.Filepath, Contents: vdir.Readme.Contents}
		}
		return s.serveModulePage(ctx, w, r, ds, &vdir.ModuleInfo, readme, info.requestedVersion)
	case vdir.Package != nil:
		return s.servePackagePage(ctx, w, r, ds, vdir, info.requestedVersion)
	default:
		return s.serveDirectoryPage(ctx, w, r, ds, vdir, info.requestedVersion)
	}
}

// legacyServeDetailsPage serves a details page for a path using the packages,
// modules, licenses and imports tables.
func (s *Server) legacyServeDetailsPage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, info *urlPathInfo) (err error) {
	defer derrors.Wrap(&err, "legacyServeDetailsPage(w, r, %v)", info)
	if info.isModule {
		return s.legacyServeModulePage(w, r, ds, info.fullPath, info.requestedVersion, info.resolvedVersion)
	}
	return s.legacyServePackagePage(w, r, ds, info.fullPath, info.modulePath, info.requestedVersion, info.resolvedVersion)
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
	if !isSupportedVersion(ctx, fullPath, requestedVersion) {
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
func isSupportedVersion(ctx context.Context, fullPath, requestedVersion string) bool {
	if stdlib.Contains(fullPath) && requestedVersion == internal.MasterVersion {
		return false
	}
	if requestedVersion == internal.LatestVersion || semver.IsValid(requestedVersion) {
		return true
	}
	if isActivePathAtMaster(ctx) {
		return requestedVersion == internal.MasterVersion
	}
	return false
}

// isActveUseDirectories reports whether the experiment for reading from the
// paths-based data model is active.
func isActiveUseDirectories(ctx context.Context) bool {
	return experiment.IsActive(ctx, internal.ExperimentUseDirectories)
}

// isActivePathAtMaster reports whether the experiment for viewing packages at
// master is active.
func isActivePathAtMaster(ctx context.Context) bool {
	return experiment.IsActive(ctx, internal.ExperimentMasterVersion) &&
		isActiveFrontendFetch(ctx)
}

// pathNotFoundError returns an error page with instructions on how to
// add a package or module to the site. pathType is always either the string
// "package" or "module".
func pathNotFoundError(ctx context.Context, pathType, fullPath, requestedVersion string) error {
	if isActiveFrontendFetch(ctx) {
		return pathNotFoundErrorNew(fullPath, requestedVersion)
	}
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(`<h3 class="Error-message">404 Not Found</h3>
				 <p class="Error-message">
				   If you think this is a valid {{.}} path, you can try fetching it following
				   the <a href="/about#adding-a-package">instructions here</a>.
				</p>`),
			MessageData: pathType,
		},
	}
}

// pathNotFoundErrorNew returns an error page that provides the user with an
// option to fetch a path.
func pathNotFoundErrorNew(fullPath, requestedVersion string) error {
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
			messageTemplate: template.MakeTrustedTemplate(`
				<h3 class="NotFound-message">Oops! {{.}} does not exist.</h3>
				<p class="NotFound-message js-notFoundMessage">
					Check that you entered it correctly, or request to fetch it.
				</p>`),
			MessageData: path,
		},
	}
}

// pathFoundAtLatestError returns an error page when the fullPath exists, but
// the version that is requested does not.
func pathFoundAtLatestError(ctx context.Context, pathType, fullPath, requestedVersion string) error {
	if isActiveFrontendFetch(ctx) {
		return pathNotFoundErrorNew(fullPath, requestedVersion)
	}
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(`
				<h3 class="Error-message">{{.TType}} {{.Path}}@{{.Version}} is not available.</h3>
				<p class="Error-message">
				  There are other versions of this {{.Type}} that are! To view them,
				  <a href="/{{.Path}}?tab=versions">click here</a>.
				</p>`),
			MessageData: struct{ TType, Type, Path, Version string }{
				strings.Title(pathType), pathType, fullPath, displayVersion(requestedVersion, fullPath)},
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

func (s *Server) servePathNotFoundPage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, fullPath, modulePath, requestedVersion, pathType string) (err error) {
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

	if isActiveFrontendFetch(ctx) && !stdlib.Contains(fullPath) {
		db, ok := ds.(*postgres.DB)
		if !ok {
			return pathNotFoundError(ctx, pathType, fullPath, requestedVersion)
		}
		modulePaths, err := candidateModulePaths(fullPath)
		if err != nil {
			return pathNotFoundError(ctx, pathType, fullPath, requestedVersion)
		}
		results := s.checkPossibleModulePaths(ctx, db, fullPath, requestedVersion, modulePaths, false)
		for _, fr := range results {
			if fr.status == statusNotFoundInVersionMap {
				// If the result is statusNotFoundInVersionMap, it means that
				// we haven't attempted to fetch this path before. Return an
				// error page giving the user the option to fetch the path.
				return pathNotFoundErrorNew(fullPath, requestedVersion)
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

	if requestedVersion == internal.LatestVersion || isActiveFrontendFetch(ctx) {
		// We already know that the fullPath does not exist at any version.
		//
		// If frontend fetch is enabled always show the 404 page so that the
		// user can request the version that they want.
		return pathNotFoundError(ctx, pathType, fullPath, requestedVersion)
	}
	// If frontend fetch is not enabled and we couldn't find a path at the
	// given version, but if there's one at the latest version we can provide a
	// link to it.
	if _, _, _, err := ds.GetPathInfo(ctx, fullPath, modulePath, internal.LatestVersion); err != nil {
		if errors.Is(err, derrors.NotFound) {
			return pathNotFoundError(ctx, pathType, fullPath, requestedVersion)
		}
		return err
	}
	return pathFoundAtLatestError(ctx, pathType, fullPath, requestedVersion)
}
