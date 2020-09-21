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
	"runtime/debug"
	"strings"

	"github.com/google/safehtml"
	"go.opencensus.io/trace"
	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
)

// A goPackage is a group of one or more Go source files with the same
// package header. Packages are part of a module.
type goPackage struct {
	path              string
	name              string
	synopsis          string
	imports           []string
	documentationHTML safehtml.HTML
	isRedistributable bool
	licenseMeta       []*licenses.Metadata // metadata of applicable licenses
	// goos and goarch are environment variables used to parse the
	// package.
	goos   string
	goarch string

	// v1path is the package path of a package with major version 1 in a given
	// series.
	v1path string
	source []byte // the source files of the package, for generating doc at serving time
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
// It matches against the given licenses to determine the subset of licenses
// that applies to each package.
// The second return value says whether any packages are "incomplete," meaning
// that they contained .go files but couldn't be processed due to current
// limitations of this site. The limitations are:
// * a maximum file size (MaxFileSize)
// * the particular set of build contexts we consider (goEnvs)
// * whether the import path is valid.
func extractPackagesFromZip(ctx context.Context, modulePath, resolvedVersion string, r *zip.Reader, d *licenses.Detector, sourceInfo *source.Info) (_ []*goPackage, _ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "extractPackagesFromZip(ctx, %q, %q, r, d)", modulePath, resolvedVersion)
	ctx, span := trace.StartSpan(ctx, "fetch.extractPackagesFromZip")
	defer span.End()
	defer func() {
		if e := recover(); e != nil {
			// The package processing code performs some sanity checks along the way.
			// None of the panics should occur, but if they do, we want to log them and
			// be able to find them. So, convert internal panics to internal errors here.
			err = fmt.Errorf("internal panic: %v\n\n%s", e, debug.Stack())
		}
	}()

	// The high-level approach is to split the processing of the zip file
	// into two phases:
	//
	// 	1. loop over all files, looking at file metadata only
	// 	2. process all files by reading their contents
	//
	// During phase 1, we populate the dirs map for each directory
	// that contains at least one .go file.

	var (
		// modulePrefix is the "<module>@<resolvedVersion>/" prefix that all files
		// are expected to have according to the zip archive layout specification
		// at the bottom of https://golang.org/cmd/go/#hdr-Module_proxy_protocol.
		modulePrefix = moduleVersionDir(modulePath, resolvedVersion) + "/"

		// dirs is the set of directories with at least one .go file,
		// to be populated during phase 1 and used during phase 2.
		//
		// The map key is the directory path, with the modulePrefix trimmed.
		// The map value is a slice of all .go files, and no other files.
		dirs = make(map[string][]*zip.File)

		// modInfo contains all the module information a package in the module
		// needs to render its documentation, to be populated during phase 1
		// and used during phase 2.
		modInfo = &dochtml.ModuleInfo{
			ModulePath:      modulePath,
			ResolvedVersion: resolvedVersion,
			ModulePackages:  make(map[string]bool),
		}

		// incompleteDirs tracks directories for which we have incomplete
		// information, due to a problem processing one of the go files contained
		// therein. We use this so that a single unprocessable package does not
		// prevent processing of other packages in the module.
		incompleteDirs       = make(map[string]bool)
		packageVersionStates = []*internal.PackageVersionState{}
	)

	// Phase 1.
	// Loop over zip files preemptively and check for problems
	// that can be detected by looking at metadata alone.
	// We'll be looking at file contents starting with phase 2 only,
	// only after we're sure this phase passed without errors.
	for _, f := range r.File {
		if f.Mode().IsDir() {
			// While "go mod download" will never put a directory in a zip, any can serve their
			// own zips. Example: go.felesatra.moe/binpack@v0.1.0.
			// Directory entries are harmless, so we just ignore them.
			continue
		}
		if !strings.HasPrefix(f.Name, modulePrefix) {
			// Well-formed module zips have all files under modulePrefix.
			return nil, nil, fmt.Errorf("expected file to have prefix %q; got = %q: %w",
				modulePrefix, f.Name, errMalformedZip)
		}
		innerPath := path.Dir(f.Name[len(modulePrefix):])
		if incompleteDirs[innerPath] {
			// We already know this directory cannot be processed, so skip.
			continue
		}
		importPath := path.Join(modulePath, innerPath)
		if ignoredByGoTool(importPath) || isVendored(importPath) {
			// File is in a directory we're not looking to process at this time, so skip it.
			continue
		}
		if !strings.HasSuffix(f.Name, ".go") {
			// We care about .go files only.
			continue
		}
		// It's possible to have a Go package in a directory that does not result in a valid import path.
		// That package cannot be imported, but that may be fine if it's a main package, intended to built
		// and run from that directory.
		// Example:  https://github.com/postmannen/go-learning/blob/master/concurrency/01-sending%20numbers%20and%20receving%20numbers%20from%20a%20channel/main.go
		// We're not set up to handle invalid import paths, so skip these packages.
		if err := module.CheckImportPath(importPath); err != nil {
			incompleteDirs[innerPath] = true
			packageVersionStates = append(packageVersionStates, &internal.PackageVersionState{
				ModulePath:  modulePath,
				PackagePath: importPath,
				Version:     resolvedVersion,
				Status:      derrors.ToStatus(derrors.PackageBadImportPath),
				Error:       err.Error(),
			})
			continue
		}
		if f.UncompressedSize64 > MaxFileSize {
			incompleteDirs[innerPath] = true
			status := derrors.ToStatus(derrors.PackageMaxFileSizeLimitExceeded)
			err := fmt.Sprintf("Unable to process %s: file size %d exceeds max limit %d",
				f.Name, f.UncompressedSize64, MaxFileSize)
			packageVersionStates = append(packageVersionStates, &internal.PackageVersionState{
				ModulePath:  modulePath,
				PackagePath: importPath,
				Version:     resolvedVersion,
				Status:      status,
				Error:       err,
			})
			continue
		}
		dirs[innerPath] = append(dirs[innerPath], f)
		if len(dirs) > maxPackagesPerModule {
			return nil, nil, fmt.Errorf("%d packages found in %q; exceeds limit %d for maxPackagePerModule", len(dirs), modulePath, maxPackagesPerModule)
		}
	}
	for pkgName := range dirs {
		modInfo.ModulePackages[path.Join(modulePath, pkgName)] = true
	}

	// Phase 2.
	// If we got this far, the file metadata was okay.
	// Start reading the file contents now to extract information
	// about Go packages.
	var pkgs []*goPackage
	for innerPath, goFiles := range dirs {
		if incompleteDirs[innerPath] {
			// Something went wrong when processing this directory, so we skip.
			log.Infof(ctx, "Skipping %q because it is incomplete", innerPath)
			continue
		}

		var (
			status error
			errMsg string
		)
		pkg, err := loadPackage(ctx, goFiles, innerPath, sourceInfo, modInfo)
		if bpe := (*BadPackageError)(nil); errors.As(err, &bpe) {
			incompleteDirs[innerPath] = true
			status = derrors.PackageInvalidContents
			errMsg = err.Error()
		} else if errors.Is(err, dochtml.ErrTooLarge) {
			status = derrors.PackageDocumentationHTMLTooLarge
			errMsg = err.Error()
		} else if err != nil {
			return nil, nil, fmt.Errorf("unexpected error loading package: %v", err)
		}

		var pkgPath string
		if pkg == nil {
			// No package.
			if len(goFiles) > 0 {
				// There were go files, but no build contexts matched them.
				incompleteDirs[innerPath] = true
				status = derrors.PackageBuildContextNotSupported
			}
			pkgPath = path.Join(modulePath, innerPath)
		} else {
			if d != nil { //  should only be nil for tests
				isRedist, lics := d.PackageInfo(innerPath)
				pkg.isRedistributable = isRedist
				for _, l := range lics {
					pkg.licenseMeta = append(pkg.licenseMeta, l.Metadata)
				}
			}
			pkgs = append(pkgs, pkg)
			pkgPath = pkg.path
		}
		code := http.StatusOK
		if status != nil {
			code = derrors.ToStatus(status)
		}
		packageVersionStates = append(packageVersionStates, &internal.PackageVersionState{
			ModulePath:  modulePath,
			PackagePath: pkgPath,
			Version:     resolvedVersion,
			Status:      code,
			Error:       errMsg,
		})
	}
	if len(pkgs) == 0 {
		return nil, packageVersionStates, errModuleContainsNoPackages
	}
	return pkgs, packageVersionStates, nil
}

// ignoredByGoTool reports whether the given import path corresponds
// to a directory that would be ignored by the go tool.
//
// The logic of the go tool for ignoring directories is documented at
// https://golang.org/cmd/go/#hdr-Package_lists_and_patterns:
//
// 	Directory and file names that begin with "." or "_" are ignored
// 	by the go tool, as are directories named "testdata".
//
func ignoredByGoTool(importPath string) bool {
	for _, el := range strings.Split(importPath, "/") {
		if strings.HasPrefix(el, ".") || strings.HasPrefix(el, "_") || el == "testdata" {
			return true
		}
	}
	return false
}

// isVendored reports whether the given import path corresponds
// to a Go package that is inside a vendor directory.
//
// The logic for what is considered a vendor directory is documented at
// https://golang.org/cmd/go/#hdr-Vendor_Directories.
func isVendored(importPath string) bool {
	return strings.HasPrefix(importPath, "vendor/") ||
		strings.Contains(importPath, "/vendor/")
}
