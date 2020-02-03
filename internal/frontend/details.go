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

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
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

func (s *Server) handleDetails(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.staticPageHandler("index.tmpl", "go.dev")(w, r)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "@", 2)
	if stdlib.Contains(parts[0]) {
		s.handleStdLib(w, r)
		return
	}
	s.handlePackageDetails(w, r)
}

// parseDetailsURLPath returns the modulePath (if known),
// pkgPath and version specified by urlPath.
// urlPath is assumed to be a valid path following the structure:
//   /<module-path>[@<version>/<suffix>]
//
// If <version> is not specified, internal.LatestVersion is used for the
// version. modulePath can only be determined if <version> is specified.
//
// Leading and trailing slashes in the urlPath are trimmed.
func parseDetailsURLPath(urlPath string) (pkgPath, modulePath, version string, err error) {
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
	if len(parts) == 1 {
		modulePath = internal.UnknownModulePath
		version = internal.LatestVersion
		pkgPath = basePath
	} else {
		// Parse the version and suffix from parts[1].
		endParts := strings.Split(parts[1], "/")
		suffix := strings.Join(endParts[1:], "/")
		version = endParts[0]
		if version == internal.LatestVersion {
			return "", "", "", fmt.Errorf("invalid version: %q", version)
		}
		if suffix == "" {
			modulePath = internal.UnknownModulePath
			pkgPath = basePath
		} else {
			modulePath = basePath
			pkgPath = basePath + "/" + suffix
		}
	}
	if err := module.CheckImportPath(pkgPath); err != nil {
		return "", "", "", fmt.Errorf("malformed path %q: %v", pkgPath, err)
	}
	if stdlib.Contains(pkgPath) {
		modulePath = stdlib.ModulePath
	}
	return pkgPath, modulePath, version, nil
}

// checkPathAndVersion verifies that the requested path and version are
// acceptable. The given path may be a module or package path.
func checkPathAndVersion(ctx context.Context, ds internal.DataSource, path, version string) (int, *errorPage) {
	if version != internal.LatestVersion && !semver.IsValid(version) {
		return http.StatusBadRequest, &errorPage{
			Message:          fmt.Sprintf("%q is not a valid semantic version.", version),
			SecondaryMessage: suggestedSearch(path),
		}
	}
	excluded, err := ds.IsExcluded(ctx, path)
	if err != nil {
		log.Errorf(ctx, "error checking excluded path: %v", err)
		return http.StatusInternalServerError, nil
	}
	if excluded {
		// Return NotFound; don't let the user know that the package was excluded.
		return http.StatusNotFound, nil
	}
	return http.StatusOK, nil
}

// servePathNotFoundErrorPage returns an error page with instructions on how to
// add a package or module to the site. pathType is always either the string
// "package" or "module".
func (s *Server) servePathNotFoundErrorPage(w http.ResponseWriter, r *http.Request, pathType string) {
	s.serveErrorPage(w, r, http.StatusNotFound, &errorPage{
		Message:          "404 Not Found",
		SecondaryMessage: template.HTML(fmt.Sprintf(`If you think this is a valid %s path, you can try fetching it following the <a href="/about#adding-a-package">instructions here</a>.`, pathType)),
	})
}
