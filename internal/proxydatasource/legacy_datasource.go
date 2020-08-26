// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxydatasource implements an internal.DataSource backed solely by a
// proxy instance.
package proxydatasource

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
)

// LegacyGetDirectory returns packages contained in the given subdirectory of a module version.
func (ds *DataSource) LegacyGetDirectory(ctx context.Context, dirPath, modulePath, version string, _ internal.FieldSet) (_ *internal.LegacyDirectory, err error) {
	defer derrors.Wrap(&err, "LegacyGetDirectory(%q, %q, %q)", dirPath, modulePath, version)

	var info *proxy.VersionInfo
	if modulePath == internal.UnknownModulePath {
		modulePath, info, err = ds.findModule(ctx, dirPath, version)
		if err != nil {
			return nil, err
		}
		version = info.Version
	}
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &internal.LegacyDirectory{
		LegacyModuleInfo: internal.LegacyModuleInfo{ModuleInfo: v.ModuleInfo},
		Path:             dirPath,
		Packages:         v.LegacyPackages,
	}, nil
}

// LegacyGetModuleLicenses returns root-level licenses detected within the module zip
// for modulePath and version.
func (ds *DataSource) LegacyGetModuleLicenses(ctx context.Context, modulePath, version string) (_ []*licenses.License, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleLicenses(%q, %q)", modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	var filtered []*licenses.License
	for _, lic := range v.Licenses {
		if !strings.Contains(lic.FilePath, "/") {
			filtered = append(filtered, lic)
		}
	}
	return filtered, nil
}

// LegacyGetPackage returns a LegacyVersionedPackage for the given pkgPath and version. If
// such a package exists in the cache, it will be returned without querying the
// proxy. Otherwise, the proxy is queried to find the longest module path at
// that version containing the package.
func (ds *DataSource) LegacyGetPackage(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.LegacyVersionedPackage, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackage(%q, %q)", pkgPath, version)

	var m *internal.Module
	if modulePath != internal.UnknownModulePath {
		m, err = ds.getModule(ctx, modulePath, version)
	} else {
		m, err = ds.getPackageVersion(ctx, pkgPath, version)
	}
	if err != nil {
		return nil, err
	}
	return packageFromVersion(pkgPath, m)
}

// LegacyGetPackageLicenses returns the Licenses that apply to pkgPath within the
// module version specified by modulePath and version.
func (ds *DataSource) LegacyGetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) (_ []*licenses.License, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackageLicenses(%q, %q, %q)", pkgPath, modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	for _, p := range v.LegacyPackages {
		if p.Path == pkgPath {
			var lics []*licenses.License
			for _, lmd := range p.Licenses {
				// lmd is just license metadata, the version has the actual licenses.
				for _, lic := range v.Licenses {
					if lic.FilePath == lmd.FilePath {
						lics = append(lics, lic)
						break
					}
				}
			}
			return lics, nil
		}
	}
	return nil, fmt.Errorf("package %s is missing from module %s: %w", pkgPath, modulePath, derrors.NotFound)
}

// LegacyGetPackagesInModule returns LegacyPackages contained in the module zip corresponding to modulePath and version.
func (ds *DataSource) LegacyGetPackagesInModule(ctx context.Context, modulePath, version string) (_ []*internal.LegacyPackage, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackagesInModule(%q, %q)", modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return v.LegacyPackages, nil
}

// LegacyGetPsuedoVersionsForModule returns versions from the the proxy /list
// endpoint, if they are pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) LegacyGetPsuedoVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetPsuedoVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, true)
}

// LegacyGetPsuedoVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) LegacyGetPsuedoVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetPsuedoVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, true)
}

// LegacyGetTaggedVersionsForModule returns versions from the the proxy /list
// endpoint, if they are tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) LegacyGetTaggedVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetTaggedVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, false)
}

// LegacyGetTaggedVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) LegacyGetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetTaggedVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, false)
}

// LegacyGetModuleInfo returns the LegacyModuleInfo as fetched from the proxy for module
// version specified by modulePath and version.
func (ds *DataSource) LegacyGetModuleInfo(ctx context.Context, modulePath, version string) (_ *internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleInfo(%q, %q)", modulePath, version)
	m, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &m.LegacyModuleInfo, nil
}
