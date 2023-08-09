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
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
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
}

type userError struct {
	userMessage string
	err         error
}

func (e *userError) Error() string {
	return e.err.Error()
}

func (e *userError) Unwrap() error {
	return e.err
}

// extractURLPathInfo extracts information from a request to pkg.go.dev.
// If an error is returned, the user will be served an http.StatusBadRequest.
func extractURLPathInfo(urlPath string) (_ *urlPathInfo, err error) {
	defer derrors.Wrap(&err, "extractURLPathInfo(%q)", urlPath)

	if m, _, _ := strings.Cut(strings.TrimPrefix(urlPath, "/"), "@"); stdlib.Contains(m) {
		return parseStdLibURLPath(urlPath)
	}
	return parseDetailsURLPath(urlPath)
}

// parseDetailsURLPath parses a URL path that refers (or may refer) to something
// in the Go ecosystem.
//
// After trimming leading and trailing slashes, the path is expected to have one
// of three forms, and we divide it into three parts: a full path, a module
// path, and a version.
//
//  1. The path has no '@', like github.com/hashicorp/vault/api.
//     This is the full path. The module path is unknown. So is the version, so we
//     treat it as the latest version for whatever the path denotes.
//
//  2. The path has "@version" at the end, like github.com/hashicorp/vault/api@v1.2.3.
//     We split this at the '@' into a full path (github.com/hashicorp/vault/api)
//     and version (v1.2.3); the module path is still unknown.
//
//  3. The path has "@version" in the middle, like github.com/hashicorp/vault@v1.2.3/api.
//     (We call this the "canonical" form of a path.)
//     We remove the version to get the full path, which is again
//     github.com/hashicorp/vault/api. The version is v1.2.3, and the module path is
//     the part before the '@', github.com/hashicorp/vault.
//
// In one case, we do a little more than parse the urlPath into parts: if the full path
// could be a part of the standard library (because it has no '.'), we assume it
// is and set the modulePath to indicate the standard library.
func parseDetailsURLPath(urlPath string) (_ *urlPathInfo, err error) {
	defer derrors.Wrap(&err, "parseDetailsURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<module-path>[/<suffix>]
	// or
	//   /<module-path>, @<version>/<suffix>
	// or
	//  /<module-path>/<suffix>, @<version>
	modulePath, rest, found := strings.Cut(urlPath, "@")
	info := &urlPathInfo{
		fullPath:         strings.TrimSuffix(strings.TrimPrefix(modulePath, "/"), "/"),
		modulePath:       internal.UnknownModulePath,
		requestedVersion: version.Latest,
	}
	if found {
		// The urlPath contains a "@". Parse the version and suffix from
		// parts[1], the string after the '@'.
		endParts := strings.Split(rest, "/")

		// Parse the requestedVersion from the urlPath.
		// The first path component after the '@' is the version.
		// You cannot explicitly write "latest" for the version.
		if endParts[0] == version.Latest {
			return nil, &userError{
				err:         fmt.Errorf("invalid version: %q", info.requestedVersion),
				userMessage: fmt.Sprintf("%q is not a valid version", endParts[0]),
			}
		}
		info.requestedVersion = endParts[0]

		// Parse the suffix following the "@version" from the urlPath.
		suffix := strings.Join(endParts[1:], "/")
		if suffix != "" {
			// If "@version" occurred in the middle of the path, the part before it
			// is the module path.
			info.modulePath = info.fullPath
			info.fullPath = info.fullPath + "/" + suffix
		}
	}
	if !isValidPath(info.fullPath) {
		return nil, &userError{
			err:         fmt.Errorf("isValidPath(%q) is false", info.fullPath),
			userMessage: fmt.Sprintf("%q is not a valid import path", info.fullPath),
		}
	}
	return info, nil
}

func parseStdLibURLPath(urlPath string) (_ *urlPathInfo, err error) {
	defer derrors.Wrap(&err, "parseStdLibURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<path>@<tag> or /<path>
	fullPath, tag, found := strings.Cut(urlPath, "@")
	fullPath = strings.TrimSuffix(strings.TrimPrefix(fullPath, "/"), "/")
	if !isValidPath(fullPath) {
		return nil, &userError{
			err:         fmt.Errorf("isValidPath(%q) is false", fullPath),
			userMessage: fmt.Sprintf("%q is not a valid import path", fullPath),
		}
	}

	info := &urlPathInfo{
		fullPath:   fullPath,
		modulePath: stdlib.ModulePath,
	}
	if !found {
		info.requestedVersion = version.Latest
		return info, nil
	}
	tag = strings.TrimSuffix(tag, "/")
	info.requestedVersion = stdlib.VersionForTag(tag)
	if info.requestedVersion == "" {
		if tag == fetch.LocalVersion {
			// Special case: 0.0.0 is the version for a local stdlib
			info.requestedVersion = fetch.LocalVersion
			return info, nil
		}
		return nil, &userError{
			err:         fmt.Errorf("invalid Go tag for url: %q", urlPath),
			userMessage: fmt.Sprintf("%q is not a valid tag for the standard library", tag),
		}
	}
	return info, nil
}

// isValidPath reports whether a requested path could be a valid unit.
func isValidPath(fullPath string) bool {
	if err := module.CheckImportPath(fullPath); err != nil {
		return false
	}
	parts := strings.Split(fullPath, "/")
	if parts[0] == "golang.org" {
		if len(parts) < 2 {
			return false
		}
		switch parts[1] {
		case "dl":
			return true
		case "x":
			return len(parts) >= 3
		default:
			return false
		}
	}
	if internal.VCSHostWithThreeElementRepoName(parts[0]) && len(parts) < 3 {
		return false
	}
	return true
}

func checkExcluded(ctx context.Context, ds internal.DataSource, fullPath string) error {
	db, ok := ds.(internal.PostgresDB)
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
	if stdlib.Contains(fullPath) && stdlib.SupportedBranches[requestedVersion] {
		return true
	}
	if _, ok := internal.DefaultBranches[requestedVersion]; ok {
		return !stdlib.Contains(fullPath) || requestedVersion == "master"
	}
	return requestedVersion == version.Latest || semver.IsValid(requestedVersion)
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
