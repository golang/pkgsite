// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"errors"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/version"
)

// LatestModuleVersions uses the proxy to get information about the latest
// versions of modulePath. It returns a LatestModuleVersions whose RawVersion
// and CookedVersion is obtained from the proxy @v/list and @latest endpoints.
// The cooked version is computed by choosing the latest version after removing
// versions that are retracted in the go.mod file of the raw version.
//
// The GoodVersion of LatestModuleVersions is not set. It should be determined
// when inserting into a data source, since it depends on the contents of the
// data source.
//
// The hasGoMod function that is passed in should check if version v of the
// module has a go.mod file, using a source other than the proxy (e.g. a
// database). If it doesn't have enough information to decide, it should return
// an error that wraps derrors.NotFound.
//
// If a module has no tagged versions and hasn't been accessed at a
// pseudo-version in a while, then the proxy's list endpoint will serve nothing
// and its @latest endpoint will return a 404/410. (Example:
// cloud.google.com/go/compute/metadata, which has a
// v0.0.0-20181107005212-dafb9c8d8707 that @latest does not return.) That is not
// a failure, but a valid state in which there is no version information for a
// module, even though particular pseudo-versions of the module might exist. In
// this case, LatestModuleVersions returns (nil, nil).
func LatestModuleVersions(ctx context.Context, modulePath string, prox *proxy.Client, hasGoMod func(v string) (bool, error)) (info *internal.LatestModuleVersions, err error) {
	defer derrors.WrapStack(&err, "LatestModuleVersions(%q)", modulePath)

	defer func() {
		if info != nil {
			log.Debugf(ctx, "LatestModuleVersions(%q) => (raw=%q cooked=%q, %v)", modulePath, info.RawVersion, info.CookedVersion, err)
		}
	}()

	// Remember calls to hasGoMod because they can be expensive.
	hasGoModResults := map[string]bool{}
	hasGoModFunc := func(v string) (bool, error) {
		result, ok := hasGoModResults[v]
		if ok {
			return result, nil
		}
		err := derrors.NotFound
		if hasGoMod != nil {
			result, err = hasGoMod(v)
		}
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return false, err
		}
		if err != nil {
			// hasGoMod doesn't know; download the zip.
			zr, err := prox.Zip(ctx, modulePath, v)
			if err != nil {
				return false, err
			}
			result = hasGoModFile(zr, modulePath, v)
		}
		hasGoModResults[v] = result
		return result, nil
	}

	// Get the raw latest version.
	versions, err := prox.Versions(ctx, modulePath)
	if err != nil {
		return nil, err
	}
	latestInfo, err := prox.Info(ctx, modulePath, internal.LatestVersion)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
	} else {
		versions = append(versions, latestInfo.Version)
	}
	if len(versions) == 0 {
		// No tagged versions, and @latest returns NotFound: no version information.
		return nil, nil
	}
	rawLatest, err := version.Latest(versions, hasGoModFunc)
	if err != nil {
		return nil, err
	}

	// Get the go.mod file at the raw latest version.
	modBytes, err := prox.Mod(ctx, modulePath, rawLatest)
	if err != nil {
		return nil, err
	}
	lmv, err := internal.NewLatestModuleVersions(modulePath, rawLatest, "", "", modBytes)
	if err != nil {
		return nil, err
	}

	// Get the cooked latest version by disallowing retracted versions.
	unretractedVersions := version.RemoveIf(versions, lmv.IsRetracted)
	lmv.CookedVersion, err = version.Latest(unretractedVersions, hasGoModFunc)
	if err != nil {
		return nil, err
	}
	return lmv, nil
}
