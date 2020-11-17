// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

type urlPathInfo struct {
	// fullPath is the full import path corresponding to the requested
	// package/module/directory page.
	fullPath string
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

// extractURLPathInfo extracts information from a request to pkg.go.dev.
// If an error is returned, the user will be served an http.StatusBadRequest.
func extractURLPathInfo(urlPath string) (_ *urlPathInfo, err error) {
	defer derrors.Wrap(&err, "extractURLPathInfo(%q)", urlPath)

	info := &urlPathInfo{}
	if parts := strings.SplitN(strings.TrimPrefix(urlPath, "/"), "@", 2); stdlib.Contains(parts[0]) {
		info.fullPath, info.requestedVersion, err = parseStdLibURLPath(urlPath)
		info.modulePath = stdlib.ModulePath
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

// validatePathAndVersion verifies that the requested path and version are
// acceptable. The given path may be a module or package path.
func validatePathAndVersion(ctx context.Context, ds internal.DataSource, fullPath, requestedVersion string) error {
	if !isSupportedVersion(fullPath, requestedVersion) {
		return invalidVersionError(fullPath, requestedVersion)
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
