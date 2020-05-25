// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DetailsPage contains data for a package of module details template.
type DetailsPage struct {
	basePage
	Title          string
	CanShowDetails bool
	Settings       TabSettings
	Details        interface{}
	Header         interface{}
	BreadcrumbPath template.HTML
	Tabs           []TabSettings

	// PageType is either "mod", "dir", or "pkg" depending on the details
	// handler.
	PageType string
}

func (s *Server) serveDetails(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		s.staticPageHandler("index.tmpl", "")(w, r)
		return nil
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "@", 2)
	if stdlib.Contains(parts[0]) {
		return s.serveStdLib(w, r)
	}
	return s.servePackageDetails(w, r)
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
func parseDetailsURLPath(urlPath string) (fullPath, modulePath, version string, err error) {
	defer derrors.Wrap(&err, "parseDetailsURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<module-path>[/<suffix>]
	// or
	//   /<module-path>, @<version>/<suffix>
	// or
	//  /<module-path>/<suffix>, @<version>
	// TODO(b/140191811) The last URL route should redirect.
	parts := strings.SplitN(urlPath, "@", 2)
	basePath := strings.TrimSuffix(strings.TrimPrefix(parts[0], "/"), "/")
	if len(parts) == 1 { // no '@'
		modulePath = internal.UnknownModulePath
		version = internal.LatestVersion
		fullPath = basePath
	} else {
		// Parse the version and suffix from parts[1], the string after the '@'.
		endParts := strings.Split(parts[1], "/")
		suffix := strings.Join(endParts[1:], "/")
		// The first path component after the '@' is the version.
		version = endParts[0]
		// You cannot explicitly write "latest" for the version.
		if version == internal.LatestVersion {
			return "", "", "", fmt.Errorf("invalid version: %q", version)
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
	return fullPath, modulePath, version, nil
}

// checkPathAndVersion verifies that the requested path and version are
// acceptable. The given path may be a module or package path.
func checkPathAndVersion(ctx context.Context, ds internal.DataSource, path, version string) error {
	if !isSupportedVersion(ctx, version) {
		return &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				Message:          fmt.Sprintf("%q is not a valid semantic version.", version),
				SecondaryMessage: suggestedSearch(path),
			},
		}
	}
	excluded, err := ds.IsExcluded(ctx, path)
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
func isSupportedVersion(ctx context.Context, version string) bool {
	if version == internal.LatestVersion || semver.IsValid(version) {
		return true
	}
	if isActivePathAtMaster(ctx) {
		return version == internal.MasterVersion
	}
	return false
}

func isActivePathAtMaster(ctx context.Context) bool {
	return experiment.IsActive(ctx, internal.ExperimentFrontendPackageAtMaster) &&
		experiment.IsActive(ctx, internal.ExperimentFrontendFetch) &&
		experiment.IsActive(ctx, internal.ExperimentInsertDirectories) &&
		experiment.IsActive(ctx, internal.ExperimentUseDirectories)
}

// pathNotFoundError returns an error page with instructions on how to
// add a package or module to the site. pathType is always either the string
// "package" or "module".
func pathNotFoundError(ctx context.Context, pathType, fullPath, version string) error {
	if isActiveFrontendFetch(ctx) {
		return pathNotFoundErrorNew(fullPath, version)
	}
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			Message:          "404 Not Found",
			SecondaryMessage: template.HTML(fmt.Sprintf(`If you think this is a valid %s path, you can try fetching it following the <a href="/about#adding-a-package">instructions here</a>.`, pathType)),
		},
	}
}

// pathNotFoundErrorNew returns an error page that provides the user with an
// option to fetch a path.
func pathNotFoundErrorNew(fullPath, version string) error {
	path := fmt.Sprintf("%s@%s", fullPath, version)
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			template:         "notfound.tmpl",
			Message:          fmt.Sprintf("Oops! %q does not exist.", path),
			SecondaryMessage: template.HTML("Check that you entered it correctly, or request to fetch it."),
		},
	}
}
