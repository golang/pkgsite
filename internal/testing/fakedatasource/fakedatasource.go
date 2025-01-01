// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fakedatasource provides a fake implementation of the internal.DataSource interface.
package fakedatasource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/version"
)

var errNotImplemented = fmt.Errorf("not implemented: %w", derrors.Unsupported)

// FakeDataSource provides a fake implementation of the internal.DataSource interface.
type FakeDataSource struct {
	modules    map[module.Version]*internal.Module
	importedBy map[string][]string
}

// New returns an initialized FakeDataSource.
func New() *FakeDataSource {
	return &FakeDataSource{
		modules:    make(map[module.Version]*internal.Module),
		importedBy: make(map[string][]string),
	}
}

// InsertModule adds the module to the FakeDataSource.
func (ds *FakeDataSource) MustInsertModule(ctx context.Context, m *internal.Module) {
	_, err := ds.InsertModule(ctx, m, nil)
	if err != nil {
		panic(fmt.Errorf("error returned by InsertModule: %w", err))
	}
}

// compareLicenses reports whether i < j according to our license sorting
// semantics. This is what the postgres database uses to sort licenses.
func compareLicenses(i, j *licenses.Metadata) bool {
	if len(strings.Split(i.FilePath, "/")) > len(strings.Split(j.FilePath, "/")) {
		return true
	}
	return i.FilePath < j.FilePath
}

func sameLicense(a, b licenses.Metadata) bool {
	return a.FilePath == b.FilePath
}

func (ds *FakeDataSource) populateUnitSubdirectories(u *internal.Unit, m *internal.Module) {
	p := u.Path + "/"
	for _, u2 := range m.Units {
		if strings.HasPrefix(u2.Path, p) || u.Path == "std" {
			var syn string
			if len(u2.Documentation) > 0 {
				syn = u2.Documentation[0].Synopsis
			}
			u.Subdirectories = append(u.Subdirectories, &internal.PackageMeta{
				Path:              u2.Path,
				Name:              u2.Name,
				Synopsis:          syn,
				IsRedistributable: u2.IsRedistributable,
				Licenses:          u2.Licenses,
			})
		}
	}
}

// compareVersion returns -1 if a's version is less than b's, 0 if they're the same
// and 1 if a's version is greater than b's.
// It panics if they don't have the same module path with the major version
// suffix removed.
func compareVersion(a, b *internal.ModuleInfo) int {
	aprefix, asuffix, _ := module.SplitPathVersion(a.ModulePath)
	bprefix, bsuffix, _ := module.SplitPathVersion(b.ModulePath)
	if aprefix != bprefix {
		panic("compareVersion called for two modules with different paths")
	}

	if asuffix == bsuffix {
		return semver.Compare(a.Version, b.Version)
	}
	return semver.Compare(module.PathMajorPrefix(asuffix), module.PathMajorPrefix(bsuffix))
}

// GetNestedModules returns the latest major version of all nested modules
// given a modulePath path prefix.
func (ds *FakeDataSource) GetNestedModules(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	latest := map[string]*internal.ModuleInfo{}
	for _, mod := range ds.modules {
		if mod.ModulePath != modulePath && !strings.HasPrefix(mod.ModulePath, modulePath+"/") {
			continue
		}

		prefix, _, _ := module.SplitPathVersion(mod.ModulePath)
		curlatest, ok := latest[prefix]
		if !ok {
			latest[prefix] = &mod.ModuleInfo
			continue
		}
		if compareVersion(&mod.ModuleInfo, curlatest) > 0 {
			latest[prefix] = &mod.ModuleInfo
		}
	}
	var infos []*internal.ModuleInfo
	for _, info := range latest {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		prefixi, _, _ := module.SplitPathVersion(infos[i].ModulePath)
		prefixj, _, _ := module.SplitPathVersion(infos[j].ModulePath)
		return prefixi < prefixj
	})
	return infos, nil
}

// GetUnit returns information about a directory, which may also be a
// module and/or package. The module and version must both be known.
// The BuildContext selects the documentation to read.
func (ds *FakeDataSource) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet, bc internal.BuildContext) (*internal.Unit, error) {
	m := ds.getModule(um.ModulePath, um.Version)
	if m == nil {
		return nil, derrors.NotFound
	}
	u := findUnit(m, um.Path)
	if u == nil {
		return nil, fmt.Errorf("import path %s not found in module %s: %w", um.Path, um.ModulePath, derrors.NotFound)
	}
	// Return only the Documentation matching the given BuildContext, if any.
	// Since we cache the module and its units, we have to copy this unit before we modify it.
	// It can be a shallow copy, since we're only modifying the Unit.Documentation field.
	u2 := *u
	if d := matchingDoc(u.Documentation, bc); d != nil {
		u2.Documentation = []*internal.Documentation{d}
	} else {
		u2.Documentation = nil
	}
	return &u2, nil
}

// matchingDoc returns the Documentation that matches the given build context
// and comes earliest in build-context order. It returns nil if there is none.
func matchingDoc(docs []*internal.Documentation, bc internal.BuildContext) *internal.Documentation {
	var (
		dMin  *internal.Documentation
		bcMin *internal.BuildContext // sorts last
	)
	for _, d := range docs {
		dbc := d.BuildContext()
		if bc.Match(dbc) && (bcMin == nil || internal.CompareBuildContexts(dbc, *bcMin) < 0) {
			dMin = d
			bcMin = &dbc
		}
	}
	return dMin
}

// GetUnitMeta returns information about a path.
func (ds *FakeDataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	module := ds.findModule(path, requestedModulePath, requestedVersion)
	if module == nil {
		return nil, fmt.Errorf("could not find module for import path %s: %w", path, derrors.NotFound)
	}
	um := &internal.UnitMeta{
		Path:       path,
		ModuleInfo: module.ModuleInfo,
	}
	u := findUnit(module, path)
	if u == nil {
		return nil, derrors.NotFound
	}
	um.Name = u.Name
	return um, nil
}

// findModule finds the module with longest module path containing the given
// package path. It returns an error if no module is found.
func (ds *FakeDataSource) findModule(pkgPath, modulePath, version string) *internal.Module {
	if modulePath != internal.UnknownModulePath {
		return ds.getModule(modulePath, version)
	}
	pkgPath = strings.TrimLeft(pkgPath, "/")
	for _, modulePath := range internal.CandidateModulePaths(pkgPath) {
		if m := ds.getModule(modulePath, version); m != nil {
			return m
		}

	}
	return nil
}

func (ds *FakeDataSource) getModule(modulePath, vers string) *internal.Module {
	if vers == version.Latest {
		return ds.getLatestModule(modulePath)
	}

	return ds.modules[module.Version{Path: modulePath, Version: vers}]
}

func (ds *FakeDataSource) getLatestModule(modulePath string) *internal.Module {
	var latestVersion module.Version
	var latestModule *internal.Module
	for vers, mod := range ds.modules {
		if vers.Path == modulePath &&
			(latestVersion == (module.Version{}) ||
				version.Later(vers.Version, latestVersion.Version)) {
			latestVersion = vers
			latestModule = mod
			continue
		}
	}
	if latestModule == nil {
		return nil
	}
	return latestModule
}

// findUnit returns the unit with the given path in m, or nil if none.
func findUnit(m *internal.Module, path string) *internal.Unit {
	for _, u := range m.Units {
		if u.Path == path {
			return u
		}
	}
	return nil
}

// GetModuleReadme is not implemented.
func (ds *FakeDataSource) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*internal.Readme, error) {
	return nil, nil
}

// GetLatestInfo gets information about the latest versions of a unit and module.
// See LatestInfo for documentation.
func (ds *FakeDataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (latest internal.LatestInfo, err error) {
	latestModule := ds.getLatestModule(modulePath)
	if latestModule == nil {
		return internal.LatestInfo{}, fmt.Errorf("could not find module %s: %w", modulePath, derrors.NotFound)
	}
	var unitFound bool
	for _, unit := range latestModule.Units {
		if unit.Path == unitPath {
			unitFound = true
		}
	}

	// Determine MajorModulePath and MajorUnitPath
	if !strings.HasPrefix(unitPath, modulePath) {
		panic(fmt.Errorf("module path %q is not a prefix of unit path %q", modulePath, unitPath))
	}
	rel := strings.TrimPrefix(unitPath, modulePath)
	prefix, _, _ := module.SplitPathVersion(modulePath)
	var latestMajorModule *internal.Module
	for _, m := range ds.modules {
		curPrefix, _, _ := module.SplitPathVersion(m.ModulePath)
		if curPrefix != prefix {
			continue
		}
		if latestMajorModule == nil || compareVersion(&m.ModuleInfo, &latestMajorModule.ModuleInfo) > 0 {
			latestMajorModule = m
		}
	}
	if latestMajorModule == nil {
		panic(fmt.Errorf("a module exists with the module path %q at the same major version,"+
			"but we couldn't find the latest version of the module", modulePath))
	}
	majorModulePath := latestMajorModule.ModulePath
	majorUnitPath := majorModulePath // We don't set it to the unit path unless one is found
	expectedMajorUnitPath := majorModulePath + rel
	for _, unit := range latestMajorModule.Units {
		if unit.Path == expectedMajorUnitPath {
			majorUnitPath = unit.Path
		}
	}

	return internal.LatestInfo{
		MinorVersion:      latestModule.Version,
		MinorModulePath:   latestModule.ModulePath,
		UnitExistsAtMinor: unitFound,
		MajorModulePath:   majorModulePath,
		MajorUnitPath:     majorUnitPath,
	}, nil
}

// SearchSupport reports the search types supported by this datasource.
func (ds *FakeDataSource) SearchSupport() internal.SearchSupport {
	// internal/frontend.TestDetermineSearchAction depends on us returning FullSearch
	// even though it doesn't depend on the search results.
	return internal.FullSearch
}

// Search searches for packages matching the given query.
// It's a basic search of documentation synopses only enough to satisfy unit tests.
func (ds *FakeDataSource) Search(ctx context.Context, q string, opts internal.SearchOptions) (results []*internal.SearchResult, err error) {
	terms := strings.Fields(q)

	for _, m := range ds.modules {
		for _, u := range m.Units {
			var containsAllTerms bool
			if len(terms) > 0 {
				containsAllTerms = true
			}
			synopsis := ""
			for _, d := range u.Documentation {
				synopsis += d.Synopsis
			}
			for _, term := range terms {
				containsAllTerms = containsAllTerms && strings.Contains(synopsis, term)
			}
			if containsAllTerms {
				result := &internal.SearchResult{
					Name:        u.Name,
					PackagePath: u.Path,
					ModulePath:  m.ModulePath,
					Version:     m.Version,
					Synopsis:    synopsis,
					CommitTime:  m.CommitTime,
					NumResults:  1,
				}
				for _, licence := range u.Licenses {
					result.Licenses = append(result.Licenses, licence.Types...)
				}
				results = append(results, result)
			}

		}
	}
	return results, nil
}

func (ds *FakeDataSource) IsExcluded(ctx context.Context, path, version string) bool {
	return false
}

// GetImportedBy returns the set of packages importing the given pkgPath.
func (ds *FakeDataSource) GetImportedBy(ctx context.Context, pkgPath, modulePath string, limit int) (paths []string, err error) {
	importedBy := append([]string{}, ds.importedBy[pkgPath]...)
	sort.Strings(importedBy)
	if len(importedBy) > limit {
		importedBy = importedBy[:limit]
	}
	return importedBy, nil
}

func (ds *FakeDataSource) GetImportedByCount(ctx context.Context, pkgPath, modulePath string) (int, error) {
	return 0, nil
}

func (ds *FakeDataSource) GetLatestMajorPathForV1Path(ctx context.Context, v1path string) (string, int, error) {
	return "", 0, errNotImplemented
}

func (ds *FakeDataSource) GetStdlibPathsWithSuffix(ctx context.Context, suffix string) ([]string, error) {
	return nil, errNotImplemented
}

func (ds *FakeDataSource) GetSymbolHistory(ctx context.Context, packagePath, modulePath string) (*internal.SymbolHistory, error) {
	return &internal.SymbolHistory{}, nil
}

func (ds *FakeDataSource) GetVersionMap(ctx context.Context, modulePath, requestedVersion string) (*internal.VersionMap, error) {
	return nil, errNotImplemented
}

func (ds *FakeDataSource) GetVersionMaps(ctx context.Context, paths []string, requestedVersion string) ([]*internal.VersionMap, error) {
	return nil, errNotImplemented
}

// GetVersionsForPath returns a list of tagged versions sorted in
// descending semver order if any exist. If none, it returns the 10 most
// recent from a list of pseudo-versions sorted in descending semver order.
func (ds *FakeDataSource) GetVersionsForPath(ctx context.Context, path string) ([]*internal.ModuleInfo, error) {
	var infos []*internal.ModuleInfo

	for _, m := range ds.modules {
		if m.ModulePath == "std" {
			for _, u := range m.Units {
				if u.Path == path {
					infos = append(infos, &m.ModuleInfo)
					continue
				}
			}
		}
		prefix, _, _ := module.SplitPathVersion(m.ModulePath)
		if !strings.HasPrefix(path, prefix) {
			continue // different module
		}
		pathSuffix := trimSlashVersionPrefix(strings.TrimPrefix(path, prefix))
		for _, u := range m.Units {
			unitSuffix := trimSlashVersionPrefix(strings.TrimPrefix(u.Path, prefix))
			if unitSuffix == pathSuffix {
				infos = append(infos, &m.ModuleInfo)
			}
		}
	}

	// Only keep pseudoversions if we only have pseudoversions.
	var nonPseudo []*internal.ModuleInfo
	for _, info := range infos {
		if !version.IsPseudo(info.Version) {
			nonPseudo = append(nonPseudo, info)
		}
	}
	if len(nonPseudo) > 0 {
		infos = nonPseudo
	}

	sort.Slice(infos, func(i, j int) bool {
		return version.ForSorting(infos[i].Version) > version.ForSorting(infos[j].Version)
	})

	if len(nonPseudo) == 0 && len(infos) > 10 {
		infos = infos[:10]
	}

	return infos, nil
}

// trimSlashVersionPrefix trims a /vN path component prefix if one is present in path,
// and returns path unchanged otherwise.
func trimSlashVersionPrefix(path string) string {
	if !strings.HasPrefix(path, "/v") {
		return path
	}
	trimSlash := path[len("/"):]
	endOfPathComponent := strings.Index(trimSlash, "/")
	if endOfPathComponent == -1 {
		endOfPathComponent = len(trimSlash)
	}
	vComponent := trimSlash[:endOfPathComponent] // first component of the path
	if m := semver.Major(vComponent); m == "" || m != vComponent {
		return path
	}
	return trimSlash[endOfPathComponent:]

}

// InsertModule inserts m into the FakeDataSource. It is only implemented for
// lmv == nil.
func (ds *FakeDataSource) InsertModule(ctx context.Context, m *internal.Module, lmv *internal.LatestModuleVersions) (isLatest bool, err error) {
	if lmv != nil {
		return false, errNotImplemented
	}

	if m != nil {
		for _, u := range m.Units {
			ds.populateUnitSubdirectories(u, m)

			// Make license info consistent.
			if u.Licenses != nil {
				// Sort licenses as postgres database does.
				sort.Slice(u.Licenses, func(i, j int) bool {
					return compareLicenses(u.Licenses[i], u.Licenses[j])
				})
				// Make sure LicenseContents match up with Licenses
				u.LicenseContents = nil
				for _, ul := range u.Licenses {
					for _, ml := range m.Licenses {
						if sameLicense(*ul, *ml.Metadata) {
							u.LicenseContents = append(u.LicenseContents, ml)
						}
					}
				}
			}

			for _, pkg := range u.Imports {
				ds.importedBy[pkg] = append(ds.importedBy[pkg], u.Path)
			}
		}
	}

	ds.modules[module.Version{Path: m.ModulePath, Version: m.Version}] = m
	latest := ds.getLatestModule(m.ModulePath)
	if latest == nil {
		panic(fmt.Errorf("getLatestModule returned no modules for %v, even though we just inserted a module with that path", m.ModulePath))
	}
	return m == latest, nil
}

func (ds *FakeDataSource) UpsertVersionMap(ctx context.Context, vm *internal.VersionMap) error {
	return errNotImplemented
}
