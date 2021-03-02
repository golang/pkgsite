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
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// RawLatestInfo uses the proxy to get information about the raw latest version
// of modulePath. If it cannot obtain it, it returns (nil, nil).
//
// The hasGoMod function that is passed in should check if version v of the
// module has a go.mod file, using a source other than the proxy (e.g. a
// database). If it doesn't have enough information to decide, it should return
// an error that wraps derrors.NotFound.
func RawLatestInfo(ctx context.Context, modulePath string, prox *proxy.Client, hasGoMod func(v string) (bool, error)) (_ *internal.RawLatestInfo, err error) {
	defer derrors.WrapStack(&err, "RawLatestInfo(%q)", modulePath)

	// No raw latest info for std; no deprecations or retractions.
	if modulePath == stdlib.ModulePath {
		return nil, nil
	}

	v, err := fetchRawLatestVersion(ctx, modulePath, prox, hasGoMod)
	if err != nil {
		return nil, err
	}
	modBytes, err := prox.Mod(ctx, modulePath, v)
	if err != nil {
		return nil, err
	}
	return internal.NewRawLatestInfo(modulePath, v, modBytes)
}

// fetchRawLatestVersion uses the proxy to determine the latest
// version of a module independent of retractions or other modifications.
//
// This meaning of "latest" is defined at https://golang.org/ref/mod#version-queries.
// That definition does not deal with a subtlety involving
// incompatible versions. The actual definition is embodied in the go command's
// queryMatcher.filterVersions method. This code is a rewrite of that method at Go
// version 1.16
// (https://go.googlesource.com/go/+/refs/tags/go1.16/src/cmd/go/internal/modload/query.go#441).
func fetchRawLatestVersion(ctx context.Context, modulePath string, prox *proxy.Client, hasGoMod func(v string) (bool, error)) (v string, err error) {
	defer derrors.WrapStack(&err, "fetchRawLatestVersion(%q)", modulePath)

	defer func() {
		log.Debugf(ctx, "fetchRawLatestVersion(%q) => (%q, %v)", modulePath, v, err)
	}()

	// Get tagged versions from the proxy's list endpoint.
	taggedVersions, err := prox.Versions(ctx, modulePath)
	if err != nil {
		return "", err
	}
	// If there are no tagged versions, use the proxy's @latest endpoint.
	if len(taggedVersions) == 0 {
		latestInfo, err := prox.Info(ctx, modulePath, internal.LatestVersion)
		if err != nil {
			return "", err
		}
		return latestInfo.Version, nil
	}

	// Find the latest of all tagged versions.
	hasGoModFunc := func(v string) (bool, error) {
		var (
			has bool
			err error
		)
		if hasGoMod == nil {
			err = derrors.NotFound
		} else {
			has, err = hasGoMod(v)
		}
		if err == nil {
			return has, nil
		} else if !errors.Is(err, derrors.NotFound) {
			return false, err
		} else {
			// hasGoMod doesn't know; download the zip.
			zr, err := prox.Zip(ctx, modulePath, v)
			if err != nil {
				return false, err
			}
			return hasGoModFile(zr, modulePath, v), nil
		}
	}
	return version.Latest(taggedVersions, hasGoModFunc)

}
