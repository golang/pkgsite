// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
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

var ErrModuleContainsNoPackages = errors.New("module contains 0 packages")

type FetchResult struct {
	ModulePath       string
	RequestedVersion string
	ResolvedVersion  string
	MainVersion      string
	MasterVersion    string
	// HasGoMod says whether the zip contain a go.mod file. If Module (below) is non-nil, then
	// Module.HasGoMod will be the same value. But HasGoMod will be populated even if Module is nil
	// because there were problems with it, as long as we can download and read the zip.
	HasGoMod             bool
	GoModPath            string
	Status               int
	Error                error
	Module               *internal.Module
	PackageVersionStates []*internal.PackageVersionState
}

// FetchModule queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and processes the contents to return an
// *internal.Module and related information.
//
// Even if err is non-nil, the result may contain useful information, like the go.mod path.
func FetchModule(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter, sourceClient *source.Client) (fr *FetchResult) {
	fr = &FetchResult{
		ModulePath:       modulePath,
		RequestedVersion: requestedVersion,
	}
	defer derrors.Wrap(&fr.Error, "FetchModule(%q, %q)", modulePath, requestedVersion)

	err := fetchModule(ctx, fr, mg, sourceClient)
	fr.Error = err
	if err != nil {
		fr.Status = derrors.ToStatus(fr.Error)
	}
	if fr.Status == 0 {
		fr.Status = http.StatusOK
	}
	return fr
}

func fetchModule(ctx context.Context, fr *FetchResult, mg ModuleGetter, sourceClient *source.Client) error {
	info, err := GetInfo(ctx, fr.ModulePath, fr.RequestedVersion, mg)
	if err != nil {
		return err
	}
	fr.ResolvedVersion = info.Version
	commitTime := info.Time

	var contentDir fs.FS
	if fr.ModulePath == stdlib.ModulePath {
		var resolvedVersion string
		contentDir, resolvedVersion, commitTime, err = stdlib.ContentDir(fr.RequestedVersion)
		if err != nil {
			return err
		}
		// If the requested version is a branch name like "master" or "main", we cannot
		// determine the right resolved version until we start working with the repo.
		fr.ResolvedVersion = resolvedVersion
	} else {
		contentDir, err = mg.ContentDir(ctx, fr.ModulePath, fr.ResolvedVersion)
		if err != nil {
			return err
		}
	}

	// Set fr.HasGoMod as early as possible, because the go command uses it to
	// decide the latest version in some cases (see fetchRawLatestVersion in
	// this package) and all it requires is a valid zip.
	if fr.ModulePath == stdlib.ModulePath {
		fr.HasGoMod = true
	} else {
		fr.HasGoMod = hasGoModFile(contentDir)
	}

	// getGoModPath may return a non-empty goModPath even if the error is
	// non-nil, if the module version is an alternative module.
	var goModBytes []byte
	fr.GoModPath, goModBytes, err = getGoModPath(ctx, fr.ModulePath, fr.ResolvedVersion, mg)
	if err != nil {
		return err
	}

	// If there is no go.mod file in the zip, try another way to detect
	// alternative modules: compare the zip signature to a list of known ones to
	// see if this is a fork. The intent is to avoid processing certain known
	// large modules, not to find every fork.
	if !fr.HasGoMod {
		forkedModule, err := forkedFrom(contentDir, fr.ModulePath, fr.ResolvedVersion)
		if err != nil {
			return err
		}
		if forkedModule != "" {
			return fmt.Errorf("forked from %s: %w", forkedModule, derrors.AlternativeModule)
		}
	}

	mod, pvs, err := processModuleContents(ctx, fr.ModulePath, fr.ResolvedVersion, fr.RequestedVersion, commitTime, contentDir, sourceClient)
	if err != nil {
		return err
	}
	mod.HasGoMod = fr.HasGoMod
	if goModBytes != nil {
		if err := processGoModFile(goModBytes, mod); err != nil {
			return fmt.Errorf("%v: %w", err.Error(), derrors.BadModule)
		}
	}
	fr.Module = mod
	fr.PackageVersionStates = pvs
	for _, state := range fr.PackageVersionStates {
		if state.Status != http.StatusOK {
			fr.Status = derrors.ToStatus(derrors.HasIncompletePackages)
		}
	}
	return nil
}

// GetInfo returns the result of a request to the proxy .info endpoint. If
// the modulePath is "std", a request to @master will return an empty
// commit time.
func GetInfo(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter) (_ *proxy.VersionInfo, err error) {
	if modulePath == stdlib.ModulePath {
		var resolvedVersion string
		resolvedVersion, err = stdlib.ZipInfo(requestedVersion)
		if err != nil {
			return nil, err
		}
		return &proxy.VersionInfo{Version: resolvedVersion}, nil
	}
	return mg.Info(ctx, modulePath, requestedVersion)
}

// getGoModPath returns the module path from the go.mod file, as well as the
// contents of the file obtained from the module getter. If modulePath is the
// standard library, then the contents will be nil.
func getGoModPath(ctx context.Context, modulePath, resolvedVersion string, mg ModuleGetter) (string, []byte, error) {
	if modulePath == stdlib.ModulePath {
		return stdlib.ModulePath, nil, nil
	}
	goModBytes, err := mg.Mod(ctx, modulePath, resolvedVersion)
	if err != nil {
		return "", nil, err
	}
	goModPath := modfile.ModulePath(goModBytes)
	if goModPath == "" {
		return "", nil, fmt.Errorf("go.mod has no module path: %w", derrors.BadModule)
	}
	if goModPath != modulePath {
		// The module path in the go.mod file doesn't match the path of the
		// zip file. Don't insert the module. Store an AlternativeModule
		// status in module_version_states.
		return goModPath, goModBytes, fmt.Errorf("module path=%s, go.mod path=%s: %w", modulePath, goModPath, derrors.AlternativeModule)
	}
	return goModPath, goModBytes, nil
}

// processModuleContents extracts information from the module filesystem.
func processModuleContents(ctx context.Context, modulePath, resolvedVersion, requestedVersion string,
	commitTime time.Time, contentDir fs.FS, sourceClient *source.Client) (_ *internal.Module, _ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "processModuleContents(%q, %q)", modulePath, resolvedVersion)

	ctx, span := trace.StartSpan(ctx, "fetch.processModuleContents")
	defer span.End()

	v := resolvedVersion
	if modulePath == stdlib.ModulePath && stdlib.SupportedBranches[requestedVersion] {
		v = requestedVersion
	}
	sourceInfo, err := source.ModuleInfo(ctx, sourceClient, modulePath, v)
	if err != nil {
		log.Infof(ctx, "error getting source info: %v", err)
	}
	readmes, err := extractReadmes(modulePath, resolvedVersion, contentDir)
	if err != nil {
		return nil, nil, err
	}
	logf := func(format string, args ...interface{}) {
		log.Infof(ctx, format, args...)
	}
	d := licenses.NewDetectorFS(modulePath, v, contentDir, logf)
	allLicenses := d.AllLicenses()
	packages, packageVersionStates, err := extractPackages(ctx, modulePath, resolvedVersion, contentDir, d, sourceInfo)
	if errors.Is(err, ErrModuleContainsNoPackages) {
		return nil, nil, fmt.Errorf("%v: %w", err.Error(), derrors.BadModule)
	}
	if err != nil {
		return nil, nil, err
	}
	return &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           resolvedVersion,
			CommitTime:        commitTime,
			IsRedistributable: d.ModuleIsRedistributable(),
			SourceInfo:        sourceInfo,
			// HasGoMod is populated by the caller.
		},
		Licenses: allLicenses,
		Units:    moduleUnits(modulePath, resolvedVersion, packages, readmes, d),
	}, packageVersionStates, nil
}

func hasGoModFile(contentDir fs.FS) bool {
	info, err := fs.Stat(contentDir, "go.mod")
	return err == nil && !info.IsDir()
}

// processGoModFile populates mod with information extracted from the contents of the go.mod file.
func processGoModFile(goModBytes []byte, mod *internal.Module) (err error) {
	defer derrors.Wrap(&err, "processGoModFile")

	mf, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return err
	}
	mod.Deprecated, mod.DeprecationComment = extractDeprecatedComment(mf)
	return nil
}

// extractDeprecatedComment looks for "Deprecated" comments in the line comments
// before the module declaration. If it finds one, it returns true along with
// the text after "Deprecated:". Otherwise it returns false, "".
func extractDeprecatedComment(mf *modfile.File) (bool, string) {
	const prefix = "Deprecated:"

	if mf.Module == nil {
		return false, ""
	}
	for _, comment := range append(mf.Module.Syntax.Before, mf.Module.Syntax.Suffix...) {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Token, "//"))
		if strings.HasPrefix(text, prefix) {
			return true, strings.TrimSpace(text[len(prefix):])
		}
	}
	return false, ""
}
