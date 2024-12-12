// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

// The ModuleGetter interface and its implementations.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fuzzy"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/tools/go/packages"
)

// ModuleGetter gets module data.
type ModuleGetter interface {
	// Info returns basic information about the module.
	Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error)

	// Mod returns the contents of the module's go.mod file.
	Mod(ctx context.Context, path, version string) ([]byte, error)

	// ContentDir returns an FS for the module's contents. The FS should match the
	// format of a module zip file's content directory. That is the
	// "<module>@<resolvedVersion>" directory that all module zips are expected
	// to have according to the zip archive layout specification at
	// https://golang.org/ref/mod#zip-files.
	ContentDir(ctx context.Context, path, version string) (fs.FS, error)

	// SourceInfo returns information about where to find a module's repo and
	// source files.
	SourceInfo(ctx context.Context, path, version string) (*source.Info, error)

	// SourceFS returns the path to serve the files of the modules loaded by
	// this ModuleGetter, and an FS that can be used to read the files. The
	// returned values are intended to be passed to
	// internal/frontend.Server.InstallFiles.
	SourceFS() (string, fs.FS)

	// String returns a representation of the getter for testing and debugging.
	String() string
}

// SearchableModuleGetter is an additional interface that may be implemented by
// ModuleGetters to support search.
type SearchableModuleGetter interface {
	// Search searches for packages matching the given query, returning at most
	// limit results.
	Search(ctx context.Context, q string, limit int) ([]*internal.SearchResult, error)
}

// VolatileModuleGetter is an additional interface that may be implemented by
// ModuleGetters to support invalidating content.
type VolatileModuleGetter interface {
	// HasChanged reports whether the referenced module has changed.
	HasChanged(context.Context, internal.ModuleInfo) (bool, error)
}

type proxyModuleGetter struct {
	prox *proxy.Client
	src  *source.Client
}

func NewProxyModuleGetter(p *proxy.Client, s *source.Client) ModuleGetter {
	return &proxyModuleGetter{p, s}
}

// Info returns basic information about the module.
func (g *proxyModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	return g.prox.Info(ctx, path, version)
}

// Mod returns the contents of the module's go.mod file.
func (g *proxyModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	return g.prox.Mod(ctx, path, version)
}

// ContentDir returns an FS for the module's contents. The FS should match the format
// of a module zip file.
func (g *proxyModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	zr, err := g.prox.Zip(ctx, path, version)
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+version)
}

// SourceInfo gets information about a module's repo and source files by calling source.ModuleInfo.
func (g *proxyModuleGetter) SourceInfo(ctx context.Context, path, version string) (*source.Info, error) {
	return source.ModuleInfo(ctx, g.src, path, version)
}

// SourceFS is unimplemented for modules served from the proxy, because we
// link directly to the module's repo.
func (g *proxyModuleGetter) SourceFS() (string, fs.FS) {
	return "", nil
}

func (g *proxyModuleGetter) String() string {
	return "Proxy"
}

// Version and commit time are pre specified when fetching a local module, as these
// fields are normally obtained from a proxy.
var (
	LocalVersion    = "v0.0.0"
	LocalCommitTime = time.Time{}
)

// A directoryModuleGetter is a ModuleGetter whose source is a directory in the file system that contains
// a module's files.
type directoryModuleGetter struct {
	modulePath string
	dir        string // absolute path to direction
}

// NewDirectoryModuleGetter returns a ModuleGetter for reading a module from a directory.
func NewDirectoryModuleGetter(modulePath, dir string) (*directoryModuleGetter, error) {
	if modulePath == "" {
		goModBytes, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil {
			return nil, fmt.Errorf("cannot obtain module path for %q (%v): %w", dir, err, derrors.BadModule)
		}
		modulePath = modfile.ModulePath(goModBytes)
		if modulePath == "" {
			return nil, fmt.Errorf("go.mod in %q has no module path: %w", dir, derrors.BadModule)
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &directoryModuleGetter{
		dir:        abs,
		modulePath: modulePath,
	}, nil
}

func (g *directoryModuleGetter) checkPath(path string) error {
	if path != g.modulePath {
		return fmt.Errorf("given module path %q does not match %q for directory %q: %w",
			path, g.modulePath, g.dir, derrors.NotFound)
	}
	return nil
}

// Info returns basic information about the module.
func (g *directoryModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	return &proxy.VersionInfo{
		Version: LocalVersion,
		Time:    LocalCommitTime,
	}, nil
}

// Mod returns the contents of the module's go.mod file.
// If the file does not exist, it returns a synthesized one.
func (g *directoryModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(g.dir, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return []byte(fmt.Sprintf("module %s\n", g.modulePath)), nil
	}
	return data, err
}

// ContentDir returns an fs.FS for the module's contents.
func (g *directoryModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	return os.DirFS(g.dir), nil
}

// SourceInfo returns a source.Info that will link to the files in the
// directory. The files will be under /files/directory/modulePath, with no
// version.
func (g *directoryModuleGetter) SourceInfo(ctx context.Context, _, _ string) (*source.Info, error) {
	return source.FilesInfo(g.fileServingPath()), nil
}

// SourceFS returns the absolute path to the directory along with a
// filesystem FS for serving the directory.
func (g *directoryModuleGetter) SourceFS() (string, fs.FS) {
	return g.fileServingPath(), os.DirFS(g.dir)
}

func (g *directoryModuleGetter) fileServingPath() string {
	return path.Join(filepath.ToSlash(g.dir), g.modulePath)
}

// For testing.
func (g *directoryModuleGetter) String() string {
	return fmt.Sprintf("Dir(%s, %s)", g.modulePath, g.dir)
}

// A goPackagesModuleGetter is a ModuleGetter whose source is go/packages.Load
// from a directory in the local file system.
type goPackagesModuleGetter struct {
	dir      string              // directory from which go/packages was run
	packages []*packages.Package // all packages
	modules  []*packages.Module  // modules references by packagages; sorted by path
	isStd    bool
}

// NewGoPackagesModuleGetter returns a ModuleGetter that loads packages using
// go/packages.Load(pattern), from the requested directory.
func NewGoPackagesModuleGetter(ctx context.Context, dir string, patterns ...string) (*goPackagesModuleGetter, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	cfg := &packages.Config{
		Context: ctx,
		Dir:     abs,
		Mode: packages.NeedName |
			packages.NeedModule |
			packages.NeedCompiledGoFiles |
			packages.NeedFiles,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	log.Infof(ctx, "go/packages.Load(%q) loaded %d packages from %s in %v", patterns, len(pkgs), dir, time.Since(start))
	if err != nil {
		return nil, err
	}

	// Collect reachable modules. Modules must be sorted for search.
	moduleSet := make(map[string]*packages.Module)
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			moduleSet[pkg.Module.Path] = pkg.Module
		}
	}
	var modules []*packages.Module
	for _, m := range moduleSet {
		modules = append(modules, m)
	}
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Path < modules[j].Path
	})

	return &goPackagesModuleGetter{
		dir:      abs,
		packages: pkgs,
		modules:  modules,
	}, nil
}

// NewGoPackagesStdlibModuleGetter returns a ModuleGetter that loads stdlib packages using
// go/packages.Load, from the requested GOROOT.
func NewGoPackagesStdlibModuleGetter(ctx context.Context, dir string) (*goPackagesModuleGetter, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	env := []string(nil)
	if dir != "" {
		env = append(os.Environ(), "GOROOT="+abs)
	}
	cfg := &packages.Config{
		Context: ctx,
		Dir:     abs,
		Mode: packages.NeedName |
			packages.NeedModule |
			packages.NeedCompiledGoFiles |
			packages.NeedFiles,
		Env: env,
	}
	pkgs, err := packages.Load(cfg, "std")
	log.Infof(ctx, "go/packages.Load(std) loaded %d packages from %s in %v", len(pkgs), dir, time.Since(start))
	if err != nil {
		return nil, err
	}

	stdmod := &packages.Module{Path: "std",
		Dir: filepath.Join(abs, "src"),
	}
	modules := []*packages.Module{stdmod}
	for _, p := range pkgs {
		p.Module = stdmod
	}

	return &goPackagesModuleGetter{
		isStd:    true,
		dir:      abs,
		packages: pkgs,
		modules:  modules,
	}, nil
}

// findModule searches known modules for a module matching the provided path.
func (g *goPackagesModuleGetter) findModule(path string) (*packages.Module, error) {
	i := sort.Search(len(g.modules), func(i int) bool {
		return g.modules[i].Path >= path
	})
	if i >= len(g.modules) || g.modules[i].Path != path {
		return nil, fmt.Errorf("%w: no module with path %q", derrors.NotFound, path)
	}
	return g.modules[i], nil
}

// Info returns basic information about the module.
//
// For invalidation of locally edited modules, the time of the resulting
// version is set to the latest mtime of a file referenced by any compiled file
// in the module.
func (g *goPackagesModuleGetter) Info(ctx context.Context, modulePath, version string) (*proxy.VersionInfo, error) {
	m, err := g.findModule(modulePath)
	if err != nil {
		return nil, err
	}
	v := LocalVersion
	if m.Version != "" {
		v = m.Version
	}
	// Note: if we ever support loading dependencies out of the module cache, we
	// may have a valid m.Time to use here.
	var t time.Time
	mtime, err := g.mtime(ctx, m)
	if err != nil {
		return nil, err
	}
	if mtime != nil {
		t = *mtime
	} else {
		t = LocalCommitTime
	}
	return &proxy.VersionInfo{
		Version: v,
		Time:    t,
	}, nil
}

// mtime returns the latest modification time of a compiled Go file contained
// in a package in the module.
//
// TODO(rfindley): we should probably walk the entire module directory, so that
// we pick up new or deleted go files, but must be careful about nested
// modules.
func (g *goPackagesModuleGetter) mtime(ctx context.Context, m *packages.Module) (*time.Time, error) {
	var mtime *time.Time
	for _, pkg := range g.packages {
		if pkg.Module != nil && pkg.Module.Path == m.Path {
			for _, f := range pkg.CompiledGoFiles {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				fi, err := os.Stat(f)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return nil, err
				}
				if mtime == nil || fi.ModTime().After(*mtime) {
					modTime := fi.ModTime()
					mtime = &modTime
				}
			}
		}
	}

	// If mtime is recent, it may be unrelable as due to system time resolution
	// we may yet receive another edit within the same tick.
	if mtime != nil && time.Since(*mtime) < 2*time.Second {
		return nil, nil
	}

	return mtime, nil
}

// Mod returns the contents of the module's go.mod file.
// If the file does not exist, it returns a synthesized one.
func (g *goPackagesModuleGetter) Mod(ctx context.Context, modulePath, version string) ([]byte, error) {
	m, err := g.findModule(modulePath)
	if err != nil {
		return nil, err
	}
	if m.Dir == "" {
		return nil, fmt.Errorf("module %q missing dir", modulePath)
	}
	data, err := os.ReadFile(filepath.Join(m.Dir, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return []byte(fmt.Sprintf("module %s\n", modulePath)), nil
	}
	return data, err
}

// ContentDir returns an fs.FS for the module's contents.
func (g *goPackagesModuleGetter) ContentDir(ctx context.Context, modulePath, version string) (fs.FS, error) {
	m, err := g.findModule(modulePath)
	if err != nil {
		return nil, err
	}
	if m.Dir == "" {
		return nil, fmt.Errorf("module %q missing dir", modulePath)
	}
	return os.DirFS(m.Dir), nil
}

// SourceInfo returns a source.Info that will link to the files in the
// directory. The files will be under /files/directory/modulePath, with no
// version.
func (g *goPackagesModuleGetter) SourceInfo(ctx context.Context, modulePath, _ string) (*source.Info, error) {
	m, err := g.findModule(modulePath)
	if err != nil {
		return nil, err
	}
	if m.Dir == "" {
		return nil, fmt.Errorf("module %q missing dir", modulePath)
	}
	p := path.Join(filepath.ToSlash(g.dir), modulePath)
	return source.FilesInfo(p), nil
}

// Open implements the fs.FS interface, matching the path name to a loaded
// module.
func (g *goPackagesModuleGetter) Open(name string) (fs.File, error) {
	var bestMatch *packages.Module
	for _, m := range g.modules {
		if strings.HasPrefix(name+"/", m.Path+"/") {
			if bestMatch == nil || m.Path > bestMatch.Path {
				bestMatch = m
			}
		}
	}
	if bestMatch == nil {
		return nil, fmt.Errorf("no module matching %s", name)
	}
	suffix := strings.TrimPrefix(name, bestMatch.Path)
	suffix = strings.TrimPrefix(suffix, "/")
	filename := filepath.Join(bestMatch.Dir, filepath.FromSlash(suffix))
	return os.Open(filename)
}

func (g *goPackagesModuleGetter) SourceFS() (string, fs.FS) {
	return filepath.ToSlash(g.dir), g
}

// For testing.
func (g *goPackagesModuleGetter) String() string {
	return fmt.Sprintf("Dir(%s)", g.dir)
}

// Search implements a crude search, using fuzzy matching to match loaded
// packages.
//
// It parses file headers to produce a synopsis of results.
func (g *goPackagesModuleGetter) Search(ctx context.Context, query string, limit int) ([]*internal.SearchResult, error) {
	matcher := fuzzy.NewSymbolMatcher(query)

	type scoredPackage struct {
		pkg   *packages.Package
		score float64
	}

	var pkgs []scoredPackage
	for _, pkg := range g.packages {
		i, score := matcher.Match([]string{pkg.PkgPath})
		if i < 0 {
			continue
		}
		pkgs = append(pkgs, scoredPackage{pkg, score})
	}

	// Sort and truncate results before parsing, to save on work.
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].score > pkgs[j].score
	})

	if len(pkgs) > limit {
		pkgs = pkgs[:limit]
	}

	var results []*internal.SearchResult
	for i, pkg := range pkgs {
		result := &internal.SearchResult{
			Name:        pkg.pkg.Name,
			PackagePath: pkg.pkg.PkgPath,
			Score:       pkg.score,
			Offset:      i,
		}
		if pkg.pkg.Module != nil {
			result.ModulePath = pkg.pkg.Module.Path
			result.Version = pkg.pkg.Module.Version
		}
		for _, file := range pkg.pkg.CompiledGoFiles {
			mode := parser.PackageClauseOnly | parser.ParseComments
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, mode)
			if err != nil {
				continue
			}
			if f.Doc != nil {
				//lint:ignore SA1019 doc.Synopsis is correct here.
				// The lint message warns that doc.Synopsis is deprecated in favor
				// of Package.Synopsis.
				// Package.Synopsis would display links on separate lines if the
				// Package were initialized with a package's code and if the synopsis
				// contained links.
				// We don't have the code and we wouldn't want the extra lines if we did,
				// so doc.Synopsis is a better choice.
				result.Synopsis = doc.Synopsis(f.Doc.Text())
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// HasChanged stats the filesystem to see if content has changed for the
// provided module. It compares the latest mtime of package files to the time
// recorded in info.CommitTime, which stores the last observed mtime.
func (g *goPackagesModuleGetter) HasChanged(ctx context.Context, info internal.ModuleInfo) (bool, error) {
	m, err := g.findModule(info.ModulePath)
	if err != nil {
		return false, err
	}
	mtime, err := g.mtime(ctx, m)
	if err != nil {
		return false, err
	}
	return mtime == nil || mtime.After(info.CommitTime), nil
}

// A stdlibZipModuleGetter gets the modules for the stdlib by downloading a zip file.
type stdlibZipModuleGetter struct {
}

// NewStdlibZipModuleGetter returns a ModuleGetter that loads stdlib packages using stdlib
// zip files.
func NewStdlibZipModuleGetter() *stdlibZipModuleGetter {
	return &stdlibZipModuleGetter{}
}

// Info returns basic information about the module.
func (g *stdlibZipModuleGetter) Info(ctx context.Context, path, vers string) (_ *proxy.VersionInfo, err error) {
	// TODO(matloob) Do we need to call stdlib.ContentDir here and get the resolved version?
	if path != "std" {
		return nil, fmt.Errorf("%w: not module std", derrors.NotFound)
	}
	var resolvedVersion string
	resolvedVersion, err = stdlib.ZipInfo(vers)
	if err != nil {
		return nil, err
	}
	return &proxy.VersionInfo{Version: resolvedVersion}, nil
}

// Mod returns the contents of the module's go.mod file.
// We return dummy contents to that include the name expected by the fetcher.
func (g *stdlibZipModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	if path != "std" {
		return nil, fmt.Errorf("%w: not module std", derrors.NotFound)
	}
	return []byte("module std\n"), nil
}

// ContentDir uses stdlib.ContentDir to return a fs.FS representing the standard library's
// contents.
func (g *stdlibZipModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	// Currently we don't actually use ContentDir and do special behavior for the stdlibZipModuleGetter.
	// TODO(matloob): stdlib.ContentDir returns information that should be returned by Info.
	// One alternative is to have Info call stdlib.ContentDir and save the results (like with a
	// singleflight) But my guess is that Info is expected to be fast, so doing that would
	// cause an unexpected slowdown.
	if path != "std" {
		return nil, fmt.Errorf("%w: not module std", derrors.NotFound)
	}
	fs, _, _, err := stdlib.ContentDir(ctx, version)
	return fs, err
}

// SourceInfo returns a source.Info that will create /files links to modules in
// the cache.
func (g *stdlibZipModuleGetter) SourceInfo(ctx context.Context, path, version string) (*source.Info, error) {
	if path != "std" {
		return nil, fmt.Errorf("%w: not module std", derrors.NotFound)
	}
	return source.NewStdlibInfo(version)
}

func (g *stdlibZipModuleGetter) SourceFS() (string, fs.FS) {
	return "", nil
}

func (g *stdlibZipModuleGetter) String() string {
	return "stdlib"
}

// A modCacheModuleGetter gets modules from a directory in the filesystem that
// is organized like the module cache, with a cache/download directory that has
// paths that correspond to proxy URLs. An example of such a directory is $(go
// env GOMODCACHE).
//
// TODO(rfindley): it would be easy and useful to add support for Search to
// this getter.
type modCacheModuleGetter struct {
	dir string
}

// NewModCacheGetter returns a ModuleGetter that reads modules from a filesystem
// directory organized like the proxy.
// If allowed is non-empty, only module@versions in allowed are served; others
// result in NotFound errors.
func NewModCacheGetter(dir string) (_ *modCacheModuleGetter, err error) {
	defer derrors.Wrap(&err, "NewFSProxyModuleGetter(%q)", dir)

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	g := &modCacheModuleGetter{dir: abs}
	return g, nil
}

// Info returns basic information about the module.
func (g *modCacheModuleGetter) Info(ctx context.Context, path, vers string) (_ *proxy.VersionInfo, err error) {
	defer derrors.Wrap(&err, "modCacheGetter.Info(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}

	// Check for a .zip file. Some directories in the download cache have .info and .mod files but no .zip.
	f, err := g.openFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	f.Close()
	data, err := g.readFile(path, vers, "info")
	if err != nil {
		return nil, err
	}
	var info proxy.VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Mod returns the contents of the module's go.mod file.
func (g *modCacheModuleGetter) Mod(ctx context.Context, path, vers string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "modCacheModuleGetter.Mod(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}

	// Check that .zip is readable.
	f, err := g.openFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	f.Close()
	return g.readFile(path, vers, "mod")
}

// ContentDir returns an fs.FS for the module's contents.
func (g *modCacheModuleGetter) ContentDir(ctx context.Context, path, vers string) (_ fs.FS, err error) {
	defer derrors.Wrap(&err, "modCacheModuleGetter.ContentDir(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}

	data, err := g.readFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+vers)
}

// SourceInfo returns a source.Info that will create /files links to modules in
// the cache.
func (g *modCacheModuleGetter) SourceInfo(ctx context.Context, mpath, version string) (*source.Info, error) {
	return source.FilesInfo(path.Join(g.dir, mpath+"@"+version)), nil
}

// SourceFS returns the absolute path to the cache, and an FS that retrieves
// files from it.
func (g *modCacheModuleGetter) SourceFS() (string, fs.FS) {
	return filepath.ToSlash(g.dir), os.DirFS(g.dir)
}

// latestVersion gets the latest version that is in the directory.
func (g *modCacheModuleGetter) latestVersion(modulePath string) (_ string, err error) {
	defer derrors.Wrap(&err, "modCacheModuleGetter.latestVersion(%q)", modulePath)

	dir, err := g.moduleDir(modulePath)
	if err != nil {
		return "", err
	}
	zips, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	if err != nil {
		return "", err
	}
	if len(zips) == 0 {
		return "", fmt.Errorf("no zips in %q for module %q: %w", g.dir, modulePath, derrors.NotFound)
	}
	var versions []string
	for _, z := range zips {
		vers := strings.TrimSuffix(filepath.Base(z), ".zip")
		versions = append(versions, vers)
	}
	return version.LatestOf(versions), nil
}

func (g *modCacheModuleGetter) readFile(path, version, suffix string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "modCacheModuleGetter.readFile(%q, %q, %q)", path, version, suffix)

	f, err := g.openFile(path, version, suffix)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (g *modCacheModuleGetter) openFile(path, version, suffix string) (_ *os.File, err error) {
	epath, err := g.escapedPath(path, version, suffix)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(epath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%w: %v", derrors.NotFound, err)
		}
		return nil, err
	}
	return f, nil
}

func (g *modCacheModuleGetter) escapedPath(modulePath, version, suffix string) (string, error) {
	dir, err := g.moduleDir(modulePath)
	if err != nil {
		return "", err
	}
	ev, err := module.EscapeVersion(version)
	if err != nil {
		return "", fmt.Errorf("version: %v: %w", err, derrors.InvalidArgument)
	}
	return filepath.Join(dir, fmt.Sprintf("%s.%s", ev, suffix)), nil
}

func (g *modCacheModuleGetter) moduleDir(modulePath string) (string, error) {
	ep, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("path: %v: %w", err, derrors.InvalidArgument)
	}
	return filepath.Join(g.dir, "cache", "download", filepath.FromSlash(ep), "@v"), nil
}

// For testing.
func (g *modCacheModuleGetter) String() string {
	return fmt.Sprintf("FSProxy(%s)", g.dir)
}
