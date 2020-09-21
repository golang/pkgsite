// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
	errMalformedZip             = errors.New("module zip is malformed")
)

type FetchResult struct {
	ModulePath           string
	RequestedVersion     string
	ResolvedVersion      string
	GoModPath            string
	Status               int
	Error                error
	Defer                func() // caller must defer this on all code paths
	Module               *internal.Module
	PackageVersionStates []*internal.PackageVersionState
}

// FetchModule queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and processes the contents to return an
// *internal.Module and related information.
//
// Even if err is non-nil, the result may contain useful information, like the go.mod path.
//
// Callers of FetchModule must
//   defer fr.Defer()
// immediately after the call.
func FetchModule(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client) (fr *FetchResult) {
	fr = &FetchResult{
		ModulePath:       modulePath,
		RequestedVersion: requestedVersion,
		Defer:            func() {},
	}
	defer func() {
		if fr.Error != nil {
			derrors.Wrap(&fr.Error, "FetchModule(%q, %q)", modulePath, requestedVersion)
			fr.Status = derrors.ToStatus(fr.Error)
		}
		if fr.Status == 0 {
			fr.Status = http.StatusOK
		}
		log.Debugf(ctx, "memory after fetch of %s@%s: %dM", modulePath, requestedVersion, allocMeg())
	}()

	var (
		commitTime time.Time
		zipReader  *zip.Reader
		zipSize    int64
		err        error
	)
	// Get the just information we need to make a load-shedding decision.
	if modulePath == stdlib.ModulePath {
		var resolvedVersion string
		resolvedVersion, zipSize, err = stdlib.ZipInfo(requestedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		fr.ResolvedVersion = resolvedVersion
	} else {
		info, err := proxyClient.GetInfo(ctx, modulePath, requestedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		fr.ResolvedVersion = info.Version
		commitTime = info.Time
		zipSize, err = proxyClient.GetZipSize(ctx, modulePath, fr.ResolvedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
	}

	// Load shed or mark module as too large.
	// We treat zip size
	// as a proxy for the total memory consumed by processing a module, and use
	// it to decide whether we can currently afford to process a module.
	shouldShed, deferFunc := zipLoadShedder.shouldShed(uint64(zipSize))
	fr.Defer = deferFunc
	if shouldShed {
		fr.Error = derrors.SheddingLoad
		return fr
	}

	if zipSize > maxModuleZipSize {
		log.Warningf(ctx, "FetchModule: %s@%s zip size %dMi exceeds max %dMi",
			modulePath, fr.ResolvedVersion, zipSize/mib, maxModuleZipSize/mib)
		fr.Error = derrors.ModuleTooLarge
		return fr
	}

	// Proceed with the fetch.
	if modulePath == stdlib.ModulePath {
		zipReader, commitTime, err = stdlib.Zip(requestedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		fr.GoModPath = stdlib.ModulePath
	} else {
		goModBytes, err := proxyClient.GetMod(ctx, modulePath, fr.ResolvedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		goModPath := modfile.ModulePath(goModBytes)
		if goModPath == "" {
			fr.Error = fmt.Errorf("go.mod has no module path: %w", derrors.BadModule)
			return fr
		}
		fr.GoModPath = goModPath
		if goModPath != modulePath {
			// The module path in the go.mod file doesn't match the path of the
			// zip file. Don't insert the module. Store an AlternativeModule
			// status in module_version_states.
			fr.Error = fmt.Errorf("module path=%s, go.mod path=%s: %w", modulePath, goModPath, derrors.AlternativeModule)
			return fr
		}
		zipReader, err = proxyClient.GetZip(ctx, modulePath, fr.ResolvedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
	}
	mod, pvs, err := processZipFile(ctx, modulePath, fr.ResolvedVersion, commitTime, zipReader, sourceClient)
	if err != nil {
		fr.Error = err
		return fr
	}
	fr.Module = mod
	fr.PackageVersionStates = pvs
	if modulePath == stdlib.ModulePath {
		fr.Module.HasGoMod = true
	}
	for _, state := range fr.PackageVersionStates {
		if state.Status != http.StatusOK {
			fr.Status = derrors.ToStatus(derrors.HasIncompletePackages)
		}
	}
	return fr
}

// processZipFile extracts information from the module version zip.
func processZipFile(ctx context.Context, modulePath string, resolvedVersion string, commitTime time.Time, zipReader *zip.Reader, sourceClient *source.Client) (_ *internal.Module, _ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "processZipFile(%q, %q)", modulePath, resolvedVersion)

	ctx, span := trace.StartSpan(ctx, "fetch.processZipFile")
	defer span.End()

	sourceInfo, err := source.ModuleInfo(ctx, sourceClient, modulePath, resolvedVersion)
	if err != nil {
		log.Infof(ctx, "error getting source info: %v", err)
	}
	readmes, err := extractReadmesFromZip(modulePath, resolvedVersion, zipReader)
	if err != nil {
		return nil, nil, fmt.Errorf("extractReadmesFromZip(%q, %q, zipReader): %v", modulePath, resolvedVersion, err)
	}
	logf := func(format string, args ...interface{}) {
		log.Infof(ctx, format, args...)
	}
	d := licenses.NewDetector(modulePath, resolvedVersion, zipReader, logf)
	allLicenses := d.AllLicenses()
	packages, packageVersionStates, err := extractPackagesFromZip(ctx, modulePath, resolvedVersion, zipReader, d, sourceInfo)
	if errors.Is(err, errModuleContainsNoPackages) || errors.Is(err, errMalformedZip) {
		return nil, nil, fmt.Errorf("%v: %w", err.Error(), derrors.BadModule)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("extractPackagesFromZip(%q, %q, zipReader, %v): %v", modulePath, resolvedVersion, allLicenses, err)
	}
	hasGoMod := zipContainsFilename(zipReader, path.Join(moduleVersionDir(modulePath, resolvedVersion), "go.mod"))

	var readmeFilePath, readmeContents string
	for _, r := range readmes {
		if path.Dir(r.Filepath) != "." {
			continue
		}
		readmeFilePath = r.Filepath
		readmeContents = r.Contents
		break
	}
	var legacyPackages []*internal.LegacyPackage
	for _, p := range packages {
		legacyPackages = append(legacyPackages, &internal.LegacyPackage{
			Path:              p.path,
			Name:              p.name,
			Synopsis:          p.synopsis,
			Imports:           p.imports,
			DocumentationHTML: p.documentationHTML,
			GOOS:              p.goos,
			GOARCH:            p.goarch,
			V1Path:            p.v1path,
			IsRedistributable: p.isRedistributable,
			Licenses:          p.licenseMeta,
		})
	}
	return &internal.Module{
		LegacyModuleInfo: internal.LegacyModuleInfo{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        modulePath,
				Version:           resolvedVersion,
				CommitTime:        commitTime,
				IsRedistributable: d.ModuleIsRedistributable(),
				HasGoMod:          hasGoMod,
				SourceInfo:        sourceInfo,
			},
			LegacyReadmeFilePath: readmeFilePath,
			LegacyReadmeContents: readmeContents,
		},
		LegacyPackages: legacyPackages,
		Licenses:       allLicenses,
		Units:          moduleUnits(modulePath, resolvedVersion, packages, readmes, d),
	}, packageVersionStates, nil
}

// moduleVersionDir formats the content subdirectory for the given
// modulePath and version.
func moduleVersionDir(modulePath, version string) string {
	return fmt.Sprintf("%s@%s", modulePath, version)
}

// zipContainsFilename reports whether there is a file with the given name in the zip.
func zipContainsFilename(r *zip.Reader, name string) bool {
	for _, f := range r.File {
		if f.Name == name {
			return true
		}
	}
	return false
}
