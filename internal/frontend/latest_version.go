// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
)

// GetLatestInfo returns various pieces of information about the latest
// versions of a unit and module:
// -  The linkable form of the minor version of the unit.
// -  The latest module path and the full unit path of any major version found given the
//    fullPath and the modulePath.
// It returns empty strings on error.
// It is intended to be used as an argument to middleware.LatestVersions.
func (s *Server) GetLatestInfo(ctx context.Context, unitPath, modulePath string) (latest middleware.LatestInfo) {
	// It is okay to use a different DataSource (DB connection) than the rest of the
	// request, because this makes self-contained calls on the DB.
	ds := s.getDataSource(ctx)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		latest.MinorVersion, err = latestMinorVersion(ctx, ds, unitPath, internal.UnknownModulePath)
		if err != nil {
			log.Errorf(ctx, "latestMinorVersion: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		latest.MajorModulePath, latest.MajorUnitPath, err = ds.GetLatestMajorVersion(ctx, unitPath, modulePath)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "GetLatestMajorVersion: %v", err)
		}
	}()

	wg.Wait()
	return latest
}

// TODO(https://github.com/golang/go/issues/40107): this is currently tested in server_test.go, but
// we should add tests for this function.
func latestMinorVersion(ctx context.Context, ds internal.DataSource, unitPath, modulePath string) (_ string, err error) {
	defer derrors.Wrap(&err, "latestMinorVersion(ctx, %q, %q)", unitPath, modulePath)
	um, err := ds.GetUnitMeta(ctx, unitPath, modulePath, internal.LatestVersion)
	if err != nil {
		return "", err
	}
	return linkVersion(um.Version, um.ModulePath), nil
}
