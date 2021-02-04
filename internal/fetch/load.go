// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

// BadPackageError represents an error loading a package
// because its contents do not make up a valid package.
//
// This can happen, for example, if the .go files fail
// to parse or declare different package names.
type BadPackageError struct {
	Err error // Not nil.
}

func (bpe *BadPackageError) Error() string { return bpe.Err.Error() }

// loadPackage loads a Go package by calling loadPackageWithBuildContext, trying
// several build contexts in turn. It returns a goPackage with documentation
// information for each build context that results in a valid package, in the
// same order that the build contexts are listed. If none of them result in a
// package, then loadPackage returns nil, nil.
//
// If a package is fine except that its documentation is too large, loadPackage
// returns a goPackage whose err field is a non-nil error with godoc.ErrTooLarge in its chain.
func loadPackage(ctx context.Context, zipGoFiles []*zip.File, innerPath string, sourceInfo *source.Info, modInfo *godoc.ModuleInfo) (_ *goPackage, err error) {
	defer derrors.Wrap(&err, "loadPackage(ctx, zipGoFiles, %q, sourceInfo, modInfo)", innerPath)
	ctx, span := trace.StartSpan(ctx, "fetch.loadPackage")
	defer span.End()

	// Make a map with all the zip file contents.
	files := make(map[string][]byte)
	for _, f := range zipGoFiles {
		_, name := path.Split(f.Name)
		b, err := readZipFile(f, MaxFileSize)
		if err != nil {
			return nil, err
		}
		files[name] = b
	}

	modulePath := modInfo.ModulePath
	importPath := path.Join(modulePath, innerPath)
	if modulePath == stdlib.ModulePath {
		importPath = innerPath
	}
	v1path := internal.V1Path(importPath, modulePath)

	var pkg *goPackage
	// Parse the package for each build context.
	// The documentation is determined by the set of matching files, so keep
	// track of those to avoid duplication.
	docsByFiles := map[string]*internal.Documentation{}
	for _, bc := range internal.BuildContexts {
		mfiles, err := matchingFiles(bc.GOOS, bc.GOARCH, files)
		if err != nil {
			return nil, err
		}
		filesKey := mapKeyForFiles(mfiles)
		if doc := docsByFiles[filesKey]; doc != nil {
			// We have seen this set of files before.
			// loadPackageWithBuildContext will produce the same outputs,
			// so don't bother calling it. Just copy the doc.
			doc2 := *doc
			doc2.GOOS = bc.GOOS
			doc2.GOARCH = bc.GOARCH
			pkg.docs = append(pkg.docs, &doc2)
			continue
		}
		name, imports, synopsis, source, err := loadPackageForBuildContext(ctx, mfiles, innerPath, sourceInfo, modInfo)
		switch {
		case errors.Is(err, derrors.NotFound):
			// No package for this build context.
			continue
		case errors.Is(err, godoc.ErrTooLarge):
			// The doc for this build context is too large. Remember that and
			// return the package for this build context; ignore the others.
			return &goPackage{
				err:     err,
				path:    importPath,
				v1path:  v1path,
				name:    name,
				imports: imports,
				docs: []*internal.Documentation{{
					GOOS:     bc.GOOS,
					GOARCH:   bc.GOARCH,
					Synopsis: synopsis,
					Source:   source,
				}},
			}, nil
		case err != nil:
			// Serious error. Fail.
			return nil, err
		default:
			// No error.
			if pkg == nil {
				pkg = &goPackage{
					path:    importPath,
					v1path:  v1path,
					name:    name,
					imports: imports, // Use the imports from the first successful build context.
				}
			}
			// All the build contexts should use the same package name. Although
			// it's technically legal for different build tags to result in different
			// package names, it's not something we support.
			if name != pkg.name {
				return nil, &BadPackageError{
					Err: fmt.Errorf("more than one package name (%q and %q)", pkg.name, name),
				}
			}
			doc := &internal.Documentation{
				GOOS:     bc.GOOS,
				GOARCH:   bc.GOARCH,
				Synopsis: synopsis,
				Source:   source,
			}
			docsByFiles[filesKey] = doc
			pkg.docs = append(pkg.docs, doc)
		}
	}
	return pkg, nil
}

// mapKeyForFiles generates a value that corresponds to the given set of file
// names and can be used as a map key.
func mapKeyForFiles(files map[string][]byte) string {
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

// httpPost allows package fetch tests to stub out playground URL fetches.
var httpPost = http.Post

// loadPackageForBuildContext loads a Go package made of .go files in
// files, which should match some build context.
// modulePath is stdlib.ModulePath for the Go standard library and the
// module path for all other modules. innerPath is the path of the Go package
// directory relative to the module root. The files argument must contain only
// .go files that have been verified to be of reasonable size and that match
// the build context.
//
// It returns the package name, list of imports, the package synopsis, and the
// serialized source (AST) for the package.
//
// It returns an error with NotFound in its chain if the directory doesn't
// contain a Go package or all .go files have been excluded by constraints. A
// *BadPackageError error is returned if the directory contains .go files but do
// not make up a valid package.
//
// If it returns an error with ErrTooLarge in its chain, the other return values
// are still valid.
func loadPackageForBuildContext(ctx context.Context, files map[string][]byte, innerPath string, sourceInfo *source.Info, modInfo *godoc.ModuleInfo) (name string, imports []string, synopsis string, source []byte, err error) {
	modulePath := modInfo.ModulePath
	defer derrors.Wrap(&err, "loadPackageWithBuildContext(files, %q, %q, %+v)", innerPath, modulePath, sourceInfo)

	packageName, goFiles, fset, err := loadFilesWithBuildContext(innerPath, files)
	if err != nil {
		return "", nil, "", nil, err
	}
	docPkg := godoc.NewPackage(fset, "", "", modInfo.ModulePackages)
	for _, pf := range goFiles {
		removeNodes := true
		// Don't strip the seemingly unexported functions from the builtin package;
		// they are actually Go builtins like make, new, etc.
		if modulePath == stdlib.ModulePath && innerPath == "builtin" {
			removeNodes = false
		}
		docPkg.AddFile(pf, removeNodes)
	}

	// Encode first, because Render messes with the AST.
	src, err := docPkg.Encode(ctx)
	if err != nil {
		return "", nil, "", nil, err
	}

	synopsis, imports, _, err = docPkg.Render(ctx, innerPath, sourceInfo, modInfo, "", "")
	if err != nil && !errors.Is(err, godoc.ErrTooLarge) {
		return "", nil, "", nil, err
	}
	return packageName, imports, synopsis, src, err
}

// loadFilesWithBuildContext loads all the given Go files at innerPath. It
// returns the package name as it occurs in the source, a map of the ASTs of all
// the Go files, and the token.FileSet used for parsing.
// If there are no non-test Go files, it returns a NotFound error.
func loadFilesWithBuildContext(innerPath string, files map[string][]byte) (pkgName string, fileMap map[string]*ast.File, _ *token.FileSet, _ error) {
	// Parse .go files and add them to the goFiles slice.
	var (
		fset            = token.NewFileSet()
		goFiles         = make(map[string]*ast.File)
		numNonTestFiles int
		packageName     string
		packageNameFile string // Name of file where packageName came from.
	)
	for name, b := range files {
		pf, err := parser.ParseFile(fset, name, b, parser.ParseComments)
		if err != nil {
			if pf == nil {
				return "", nil, nil, fmt.Errorf("internal error: the source couldn't be read: %v", err)
			}
			return "", nil, nil, &BadPackageError{Err: err}
		}
		// Remember all files, including test files for their examples.
		goFiles[name] = pf
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Keep track of the number of non-test files to check that the package name is the same.
		// and also because a directory with only test files doesn't count as a
		// Go package.
		numNonTestFiles++
		if numNonTestFiles == 1 {
			packageName = pf.Name.Name
			packageNameFile = name
		} else if pf.Name.Name != packageName {
			return "", nil, nil, &BadPackageError{Err: &build.MultiplePackageError{
				Dir:      innerPath,
				Packages: []string{packageName, pf.Name.Name},
				Files:    []string{packageNameFile, name},
			}}
		}
	}
	if numNonTestFiles == 0 {
		// This directory doesn't contain a package, or at least not one
		// that matches this build context.
		return "", nil, nil, derrors.NotFound
	}
	return packageName, goFiles, fset, nil
}

// matchingFiles returns a map from file names to their contents, read from zipGoFiles.
// It includes only those files that match the build context determined by goos and goarch.
func matchingFiles(goos, goarch string, allFiles map[string][]byte) (matchedFiles map[string][]byte, err error) {
	defer derrors.Wrap(&err, "matchingFiles(%q, %q, zipGoFiles)", goos, goarch)

	// bctx is used to make decisions about which of the .go files are included
	// by build constraints.
	bctx := &build.Context{
		GOOS:        goos,
		GOARCH:      goarch,
		CgoEnabled:  true,
		Compiler:    build.Default.Compiler,
		ReleaseTags: build.Default.ReleaseTags,

		JoinPath: path.Join,
		OpenFile: func(name string) (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(allFiles[name])), nil
		},

		// If left nil, the default implementations of these read from disk,
		// which we do not want. None of these functions should be used
		// inside this function; it would be an internal error if they are.
		// Set them to non-nil values to catch if that happens.
		SplitPathList: func(string) []string { panic("internal error: unexpected call to SplitPathList") },
		IsAbsPath:     func(string) bool { panic("internal error: unexpected call to IsAbsPath") },
		IsDir:         func(string) bool { panic("internal error: unexpected call to IsDir") },
		HasSubdir:     func(string, string) (string, bool) { panic("internal error: unexpected call to HasSubdir") },
		ReadDir:       func(string) ([]os.FileInfo, error) { panic("internal error: unexpected call to ReadDir") },
	}

	// Copy the input map so we don't modify it.
	matchedFiles = map[string][]byte{}
	for n, c := range allFiles {
		matchedFiles[n] = c
	}
	for name := range allFiles {
		match, err := bctx.MatchFile(".", name) // This will access the file we just added to files map above.
		if err != nil {
			return nil, &BadPackageError{Err: fmt.Errorf(`bctx.MatchFile(".", %q): %w`, name, err)}
		}
		if !match {
			delete(matchedFiles, name)
		}
	}
	return matchedFiles, nil
}

// readZipFile decompresses zip file f and returns its uncompressed contents.
// The caller can check f.UncompressedSize64 before calling readZipFile to
// get the expected uncompressed size of f.
//
// limit is the maximum number of bytes to read.
func readZipFile(f *zip.File, limit int64) (_ []byte, err error) {
	defer derrors.Add(&err, "readZipFile(%q)", f.Name)

	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("f.Open(): %v", err)
	}
	b, err := ioutil.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("ioutil.ReadAll(r): %v", err)
	}
	if err := r.Close(); err != nil {
		return nil, fmt.Errorf("closing: %v", err)
	}
	return b, nil
}

// mib is the number of bytes in a mebibyte (Mi).
const mib = 1024 * 1024

// The largest module zip size we can comfortably process.
// We probably will OOM if we process a module whose zip is larger.
var maxModuleZipSize int64 = math.MaxInt64

func init() {
	v := config.GetEnvInt("GO_DISCOVERY_MAX_MODULE_ZIP_MI", -1)
	if v > 0 {
		maxModuleZipSize = int64(v) * mib
	}
}

var zipLoadShedder *loadShedder

func init() {
	ctx := context.Background()
	mebis := config.GetEnvInt("GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI", -1)
	if mebis > 0 {
		log.Infof(ctx, "shedding load over %dMi", mebis)
		zipLoadShedder = &loadShedder{maxSizeInFlight: uint64(mebis) * mib}
	}
}

// ZipLoadShedStats returns a snapshot of the current LoadShedStats for zip files.
func ZipLoadShedStats() LoadShedStats {
	if zipLoadShedder != nil {
		return zipLoadShedder.stats()
	}
	return LoadShedStats{}
}
