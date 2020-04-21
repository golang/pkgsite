// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// LatestVersion returns the latest version of the package or module.
// The linkable form of the version is returned.
// It returns the empty string on error.
// It is intended to be used as an argument to middleware.LatestVersion.
func (s *Server) LatestVersion(ctx context.Context, packagePath, modulePath, pageType string) string {
	v, err := s.latestVersion(ctx, packagePath, modulePath, pageType)
	if err != nil {
		// We get NotFound errors from directories; they clutter the log.
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "GetLatestVersion: %v", err)
		}
		return ""
	}
	return v
}

func (s *Server) latestVersion(ctx context.Context, packagePath, modulePath, pageType string) (_ string, err error) {
	defer derrors.Wrap(&err, "latestVersion(ctx, %q, %q)", modulePath, packagePath)

	var mi *internal.ModuleInfo
	switch pageType {
	case "mod":
		mi, err = s.ds.GetModuleInfo(ctx, modulePath, internal.LatestVersion)
		if err != nil {
			return "", err
		}
	case "pkg":
		pkg, err := s.ds.GetPackage(ctx, packagePath, modulePath, internal.LatestVersion)
		if err != nil {
			return "", err
		}
		mi = &pkg.ModuleInfo
	default:
		// For directories we don't have a well-defined latest version.
		return "", nil
	}
	return linkVersion(mi.Version, modulePath), nil
}
