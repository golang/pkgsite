// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package urlinfo provides functions for extracting information out
// of url paths.
package urlinfo

import (
	"fmt"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// URLPathInfo contains the information about what unit is requested in a URL path.
type URLPathInfo struct {
	// FullPath is the full import path corresponding to the requested
	// package/module/directory page.
	FullPath string
	// ModulePath is the path of the module corresponding to the FullPath and
	// resolvedVersion. If unknown, it is set to internal.UnknownModulePath.
	ModulePath string
	// requestedVersion is the version requested by the user, which will be one
	// of the following: "latest", "master", a Go version tag, or a semantic
	// version.
	RequestedVersion string
}

type UserError struct {
	UserMessage string
	Err         error
}

func (e *UserError) Error() string {
	return e.Err.Error()
}

func (e *UserError) Unwrap() error {
	return e.Err
}

// ExtractURLPathInfo extracts information from a request to pkg.go.dev.
// If an error is returned, the user will be served an http.StatusBadRequest.
func ExtractURLPathInfo(urlPath string) (_ *URLPathInfo, err error) {
	defer derrors.Wrap(&err, "ExtractURLPathInfo(%q)", urlPath)

	if m, _, _ := strings.Cut(strings.TrimPrefix(urlPath, "/"), "@"); stdlib.Contains(m) {
		return parseStdlibURLPath(urlPath)
	}
	return ParseDetailsURLPath(urlPath)
}

// ParseDetailsURLPath parses a URL path that refers (or may refer) to something
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
// is and set the ModulePath to indicate the standard library.
func ParseDetailsURLPath(urlPath string) (_ *URLPathInfo, err error) {
	defer derrors.Wrap(&err, "ParseDetailsURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<module-path>[/<suffix>]
	// or
	//   /<module-path>, @<version>/<suffix>
	// or
	//  /<module-path>/<suffix>, @<version>
	modulePath, rest, found := strings.Cut(urlPath, "@")
	info := &URLPathInfo{
		FullPath:         strings.TrimSuffix(strings.TrimPrefix(modulePath, "/"), "/"),
		ModulePath:       internal.UnknownModulePath,
		RequestedVersion: version.Latest,
	}
	if found {
		// The urlPath contains a "@". Parse the version and suffix from
		// parts[1], the string after the '@'.
		endParts := strings.Split(rest, "/")

		// Parse the requestedVersion from the urlPath.
		// The first path component after the '@' is the version.
		// You cannot explicitly write "latest" for the version.
		if endParts[0] == version.Latest {
			return nil, &UserError{
				Err:         fmt.Errorf("invalid version: %q", info.RequestedVersion),
				UserMessage: fmt.Sprintf("%q is not a valid version", endParts[0]),
			}
		}
		info.RequestedVersion = endParts[0]

		// Parse the suffix following the "@version" from the urlPath.
		suffix := strings.Join(endParts[1:], "/")
		if suffix != "" {
			// If "@version" occurred in the middle of the path, the part before it
			// is the module path.
			info.ModulePath = info.FullPath
			info.FullPath = info.FullPath + "/" + suffix
		}
	}
	if !IsValidPath(info.FullPath) {
		return nil, &UserError{
			Err:         fmt.Errorf("IsValidPath(%q) is false", info.FullPath),
			UserMessage: fmt.Sprintf("%q is not a valid import path", info.FullPath),
		}
	}
	return info, nil
}

func parseStdlibURLPath(urlPath string) (_ *URLPathInfo, err error) {
	defer derrors.Wrap(&err, "parseStdlibURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<path>@<tag> or /<path>
	fullPath, tag, found := strings.Cut(urlPath, "@")
	fullPath = strings.TrimSuffix(strings.TrimPrefix(fullPath, "/"), "/")
	if !IsValidPath(fullPath) {
		return nil, &UserError{
			Err:         fmt.Errorf("IsValidPath(%q) is false", fullPath),
			UserMessage: fmt.Sprintf("%q is not a valid import path", fullPath),
		}
	}

	info := &URLPathInfo{
		FullPath:   fullPath,
		ModulePath: stdlib.ModulePath,
	}
	if !found {
		info.RequestedVersion = version.Latest
		return info, nil
	}
	tag = strings.TrimSuffix(tag, "/")
	info.RequestedVersion = stdlib.VersionForTag(tag)
	if info.RequestedVersion == "" {
		if tag == fetch.LocalVersion {
			// Special case: 0.0.0 is the version for a local stdlib
			info.RequestedVersion = fetch.LocalVersion
			return info, nil
		}
		return nil, &UserError{
			Err:         fmt.Errorf("invalid Go tag for url: %q", urlPath),
			UserMessage: fmt.Sprintf("%q is not a valid tag for the standard library", tag),
		}
	}
	return info, nil
}

// IsValidPath reports whether a requested path could be a valid unit.
func IsValidPath(fullPath string) bool {
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

// IsSupportedVersion reports whether the version is supported by the frontend.
func IsSupportedVersion(fullPath, requestedVersion string) bool {
	if stdlib.Contains(fullPath) && stdlib.SupportedBranches[requestedVersion] {
		return true
	}
	if _, ok := internal.DefaultBranches[requestedVersion]; ok {
		return !stdlib.Contains(fullPath) || requestedVersion == "master"
	}
	return requestedVersion == version.Latest || semver.IsValid(requestedVersion)
}
