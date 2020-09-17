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

// GetLatestMajorVersion returns the major version of a package or module.
// If a module isn't found from the series path or an error ocurs, an empty string is returned
// It is intended to be used as an argument to middleware.LatestVersions.
func (s *Server) GetLatestMajorVersion(ctx context.Context, seriesPath string) string {
	mv, err := s.getDataSource(ctx).GetLatestMajorVersion(ctx, seriesPath)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "GetLatestMajorVersion: %v", err)
		}
		return ""
	}

	return mv
}

// GetLatestMinorVersion returns the latest minor version of the package or module.
// The linkable form of the minor version is returned and is an empty string on error.
// It is intended to be used as an argument to middleware.LatestVersions.
func (s *Server) GetLatestMinorVersion(ctx context.Context, packagePath, modulePath, pageType string) string {
	// It is okay to use a different DataSource (DB connection) than the rest of the
	// request, because this makes a self-contained call on the DB.
	v, err := latestMinorVersion(ctx, s.getDataSource(ctx), packagePath, modulePath, pageType)
	if err != nil {
		// We get NotFound errors from directories; they clutter the log.
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "GetLatestMinorVersion: %v", err)
		}
		return ""
	}

	return v
}

// TODO(https://github.com/golang/go/issues/40107): this is currently tested in server_test.go, but
// we should add tests for this function.
func latestMinorVersion(ctx context.Context, ds internal.DataSource, packagePath, modulePath, pageType string) (_ string, err error) {
	defer derrors.Wrap(&err, "latestMinorVersion(ctx, %q, %q)", modulePath, packagePath)
	fullPath := packagePath
	if pageType == pageTypeModule || pageType == pageTypeStdLib {
		fullPath = modulePath
	}
	um, err := ds.GetUnitMeta(ctx, fullPath, modulePath, internal.LatestVersion)
	if err != nil {
		return "", err
	}
	return linkVersion(um.Version, um.ModulePath), nil
}
