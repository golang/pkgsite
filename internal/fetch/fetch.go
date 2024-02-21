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

	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/trace"
)

var ErrModuleContainsNoPackages = errors.New("module contains 0 packages")

type FetchResult struct {
	ModulePath       string
	RequestedVersion string
	ResolvedVersion  string
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

// A LazyModule contains the information needed to compute a FetchResult,
// but has only done enough work to compute the UnitMetas in the module.
// It provides a Unit method to compute a single unit or a fetchResult
// method to compute the whole FetchResult.
type LazyModule struct {
	internal.ModuleInfo
	UnitMetas        []*internal.UnitMeta
	goModPath        string
	requestedVersion string
	failedPackages   []*internal.PackageVersionState
	licenseDetector  *licenses.Detector
	contentDir       fs.FS
	godocModInfo     *godoc.ModuleInfo
	Error            error
}

// FetchModule queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and processes the contents to return an
// *internal.Module and related information.
//
// Even if err is non-nil, the result may contain useful information, like the go.mod path.
func FetchModule(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter) (fr *FetchResult) {
	lm := FetchLazyModule(ctx, modulePath, requestedVersion, mg)
	return lm.fetchResult(ctx)
}

// FetchLazyModule queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and does just enough processing to produce
// UnitMetas for all the modules. The full units are computed as needed.
func FetchLazyModule(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter) *LazyModule {
	lm, err := fetchLazyModule(ctx, modulePath, requestedVersion, mg)
	if err != nil {
		lm.Error = err
	}
	return lm
}

func fetchLazyModule(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter) (*LazyModule, error) {
	lm := &LazyModule{
		requestedVersion: requestedVersion,
	}
	lm.ModuleInfo.ModulePath = modulePath

	info, err := GetInfo(ctx, modulePath, requestedVersion, mg)
	if err != nil {
		return lm, err
	}
	lm.ModuleInfo.Version = info.Version
	commitTime := info.Time

	var contentDir fs.FS
	switch mg.(type) {
	case *stdlibZipModuleGetter:
		// Special behavior for stdlibZipModuleGetter because its info doesn't actually
		// give us the true resolved version.
		var resolvedVersion string
		contentDir, resolvedVersion, commitTime, err = stdlib.ContentDir(ctx, requestedVersion)
		if err != nil {
			return lm, err
		}
		// If the requested version is a branch name like "master" or "main", we cannot
		// determine the right resolved version until we start working with the repo.
		lm.ModuleInfo.Version = resolvedVersion
	default:
		contentDir, err = mg.ContentDir(ctx, modulePath, lm.ModuleInfo.Version)
		if err != nil {
			return lm, err
		}
	}
	lm.ModuleInfo.CommitTime = commitTime
	lm.contentDir = contentDir

	if modulePath == stdlib.ModulePath {
		lm.ModuleInfo.HasGoMod = true
	} else {
		lm.ModuleInfo.HasGoMod = hasGoModFile(contentDir)
	}

	// getGoModPath may return a non-empty goModPath even if the error is
	// non-nil, if the module version is an alternative module.
	var goModBytes []byte
	lm.goModPath, goModBytes, err = getGoModPath(ctx, modulePath, lm.ModuleInfo.Version, mg)
	if err != nil {
		return lm, err
	}

	// If there is no go.mod file in the zip, try other ways to detect
	// alternative modules:
	// 1. Compare the module path to a list of known alternative module paths.
	// 2. Compare the zip signature to a list of known ones to see if this is a
	//    fork. The intent is to avoid processing certain known large modules, not
	//    to find every fork.
	if !lm.ModuleInfo.HasGoMod {
		if modPath := knownAlternativeFor(modulePath); modPath != "" {
			return lm, fmt.Errorf("known alternative to %s: %w", modPath, derrors.AlternativeModule)
		}
		forkedModule, err := forkedFrom(contentDir, modulePath, lm.ModuleInfo.Version)
		if err != nil {
			return lm, err
		}
		if forkedModule != "" {
			return lm, fmt.Errorf("forked from %s: %w", forkedModule, derrors.AlternativeModule)
		}
	}

	// populate the rest of lm.ModuleInfo before calling extractUnitMetas with it.
	v := lm.ModuleInfo.Version // version to use for SourceInfo and licenses.NewDetectorFS
	if _, ok := mg.(*stdlibZipModuleGetter); ok {
		if modulePath == stdlib.ModulePath && stdlib.SupportedBranches[requestedVersion] {
			v = requestedVersion
		}
	}
	lm.ModuleInfo.SourceInfo, err = mg.SourceInfo(ctx, modulePath, v)
	if err != nil {
		log.Infof(ctx, "error getting source info: %v", err)
	}
	logf := func(format string, args ...any) {
		log.Infof(ctx, format, args...)
	}
	lm.licenseDetector = licenses.NewDetectorFS(modulePath, v, contentDir, logf)
	lm.ModuleInfo.IsRedistributable = lm.licenseDetector.ModuleIsRedistributable()
	lm.UnitMetas, lm.godocModInfo, lm.failedPackages, err = extractUnitMetas(ctx, lm.ModuleInfo, contentDir)
	if err != nil {
		return lm, err
	}
	if goModBytes != nil {
		if err := processGoModFile(goModBytes, &lm.ModuleInfo); err != nil {
			return lm, fmt.Errorf("%v: %w", err, derrors.BadModule)
		}
	}

	return lm, nil
}

func (lm *LazyModule) Unit(ctx context.Context, path string) (*internal.Unit, error) {
	var unitMeta *internal.UnitMeta
	for _, um := range lm.UnitMetas {
		if um.Path == path {
			unitMeta = um
		}
	}
	u, _, err := lm.unit(ctx, unitMeta)
	if err == nil && u == nil {
		return nil, fmt.Errorf("unit %v does not exist in module", path)
	}
	return u, err
}

// unit returns the Unit for the given path. It also returns a packageVersionState representing
// the state of the work of computing the Unit after the LazyModule was computed. PackageVersionStates
// representing packages that failed while the LazyModule was computed are set on the LazyModule.
func (lm *LazyModule) unit(ctx context.Context, unitMeta *internal.UnitMeta) (*internal.Unit, *internal.PackageVersionState, error) {
	readme, err := extractReadme(lm.ModulePath, unitMeta.Path, lm.ModuleInfo.Version, lm.contentDir)
	if err != nil {
		return nil, nil, err
	}
	// This unit represents the module itself, not a package.
	if !unitMeta.IsPackage() {
		return moduleUnit(lm.ModulePath, unitMeta, nil, readme, lm.licenseDetector), nil, nil
	}
	pkg, pvs, err := extractPackage(ctx, lm.ModulePath, unitMeta.Path, lm.contentDir, lm.licenseDetector, lm.SourceInfo, lm.godocModInfo)
	if err != nil || (pvs != nil && pvs.Status != 200) {
		// pvs can be non-nil even if err is non-nil.
		return nil, pvs, err
	}

	u := moduleUnit(lm.ModulePath, unitMeta, pkg, readme, lm.licenseDetector)
	return u, pvs, nil
}

func (lm *LazyModule) fetchResult(ctx context.Context) *FetchResult {
	fr := &FetchResult{
		ModulePath:       lm.ModulePath,
		RequestedVersion: lm.requestedVersion,
		ResolvedVersion:  lm.ModuleInfo.Version,
		Module: &internal.Module{
			ModuleInfo: lm.ModuleInfo,
		},
		HasGoMod:  lm.HasGoMod,
		GoModPath: lm.goModPath,
	}
	if lm.Error != nil {
		fr.Error = lm.Error
		fr.Status = derrors.ToStatus(lm.Error)
		if fr.Status == 0 {
			fr.Status = http.StatusOK
		}
		return fr
	}
	fr.Module.Licenses = lm.licenseDetector.AllLicenses()
	// We need to set HasGoMod here rather than on the ModuleInfo when
	// it's created because the ModuleInfo that goes on the units shouldn't
	// have HasGoMod set on it.
	packageVersionStates := append([]*internal.PackageVersionState{}, lm.failedPackages...)
	for _, um := range lm.UnitMetas {
		unit, pvs, err := lm.unit(ctx, um)
		if err != nil {
			fr.Error = err
		}
		if pvs != nil && um.IsPackage() {
			packageVersionStates = append(packageVersionStates, pvs)
		}
		if unit == nil {
			// No unit was produced but we still had a useful pvs.
			continue
		}
		fr.Module.Units = append(fr.Module.Units, unit)
	}
	if fr.Error != nil {
		fr.Status = derrors.ToStatus(fr.Error)
	}
	if fr.Status == 0 {
		fr.Status = http.StatusOK
	}
	fr.PackageVersionStates = packageVersionStates
	for _, state := range fr.PackageVersionStates {
		if state.Status != http.StatusOK {
			fr.Status = derrors.ToStatus(derrors.HasIncompletePackages)
		}
	}
	return fr
}

// GetInfo returns the result of a request to the proxy .info endpoint. If
// the modulePath is "std", a request to @master will return an empty
// commit time.
func GetInfo(ctx context.Context, modulePath, requestedVersion string, mg ModuleGetter) (_ *proxy.VersionInfo, err error) {
	return mg.Info(ctx, modulePath, requestedVersion)
}

// getGoModPath returns the module path from the go.mod file, as well as the
// contents of the file obtained from the module getter. If modulePath is the
// standard library, then the contents will be nil.
func getGoModPath(ctx context.Context, modulePath, resolvedVersion string, mg ModuleGetter) (string, []byte, error) {
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

// extractUnitMetas extracts UnitMeta information from the module filesystem and
// populates the LazyModule with that information and additional module-level data.
func extractUnitMetas(ctx context.Context, minfo internal.ModuleInfo,
	contentDir fs.FS) (unitMetas []*internal.UnitMeta, _ *godoc.ModuleInfo, _ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "extractUnitMetas(%q, %q)", minfo.ModulePath, minfo.Version)

	ctx, span := trace.StartSpan(ctx, "fetch.extractUnitMetas")
	defer span.End()

	packageMetas, godocModInfo, failedMetaPackages, err := extractPackageMetas(ctx, minfo.ModulePath, minfo.Version, contentDir)
	if errors.Is(err, ErrModuleContainsNoPackages) {
		return nil, nil, nil, fmt.Errorf("%v: %w", err.Error(), derrors.BadModule)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return moduleUnitMetas(minfo, packageMetas), godocModInfo, failedMetaPackages, nil
}

func hasGoModFile(contentDir fs.FS) bool {
	info, err := fs.Stat(contentDir, "go.mod")
	return err == nil && !info.IsDir()
}

// processGoModFile populates mod with information extracted from the contents of the go.mod file.
func processGoModFile(goModBytes []byte, mod *internal.ModuleInfo) (err error) {
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
