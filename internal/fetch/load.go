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
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/google/safehtml/template"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch/dochtml"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
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

// Go environments used to construct build contexts in loadPackage.
var goEnvs = []struct{ GOOS, GOARCH string }{
	{"linux", "amd64"},
	{"windows", "amd64"},
	{"darwin", "amd64"},
	{"js", "wasm"},
	{"linux", "js"},
}

// loadPackage loads a Go package by calling loadPackageWithBuildContext, trying
// several build contexts in turn. The first build context in the list to produce
// a non-empty package is used. If none of them result in a package, then
// loadPackage returns nil, nil.
//
// If the package is fine except that its documentation is too large, loadPackage
// returns both a package and a non-nil error with dochtml.ErrTooLarge in its chain.
func loadPackage(ctx context.Context, zipGoFiles []*zip.File, innerPath string, sourceInfo *source.Info, modInfo *dochtml.ModuleInfo) (*internal.LegacyPackage, error) {
	ctx, span := trace.StartSpan(ctx, "fetch.loadPackage")
	defer span.End()
	for _, env := range goEnvs {
		pkg, err := loadPackageWithBuildContext(ctx, env.GOOS, env.GOARCH, zipGoFiles, innerPath, sourceInfo, modInfo)
		if err != nil && !errors.Is(err, dochtml.ErrTooLarge) {
			return nil, err
		}
		if pkg != nil {
			return pkg, err
		}
	}
	return nil, nil
}

// httpPost allows package fetch tests to stub out playground URL fetches.
var httpPost = http.Post

const docTooLargeReplacement = `<p>Documentation is too large to display.</p>`

// loadPackageWithBuildContext loads a Go package made of .go files in zipGoFiles
// using a build context constructed from the given GOOS and GOARCH values.
// modulePath is stdlib.ModulePath for the Go standard library and the module
// path for all other modules. innerPath is the path of the Go package directory
// relative to the module root.
//
// zipGoFiles must contain only .go files that have been verified
// to be of reasonable size.
//
// The returned LegacyPackage.Licenses field is not populated.
//
// It returns a nil LegacyPackage if the directory doesn't contain a Go package
// or all .go files have been excluded by constraints.
// A *BadPackageError error is returned if the directory
// contains .go files but do not make up a valid package.
func loadPackageWithBuildContext(ctx context.Context, goos, goarch string, zipGoFiles []*zip.File, innerPath string, sourceInfo *source.Info, modInfo *dochtml.ModuleInfo) (_ *internal.LegacyPackage, err error) {
	modulePath := modInfo.ModulePath
	defer derrors.Wrap(&err, "loadPackageWithBuildContext(%q, %q, zipGoFiles, %q, %q, %+v)",
		goos, goarch, innerPath, modulePath, sourceInfo)
	// Apply build constraints to get a map from matching file names to their contents.
	files, err := matchingFiles(goos, goarch, zipGoFiles)
	if err != nil {
		return nil, err
	}

	// Parse .go files and add them to the goFiles slice.
	var (
		fset            = token.NewFileSet()
		goFiles         = make(map[string]*ast.File)
		allGoFiles      []*ast.File
		packageName     string
		packageNameFile string // Name of file where packageName came from.
	)
	for name, b := range files {
		pf, err := parser.ParseFile(fset, name, b, parser.ParseComments)
		if err != nil {
			if pf == nil {
				return nil, fmt.Errorf("internal error: the source couldn't be read: %v", err)
			}
			return nil, &BadPackageError{Err: err}
		}
		allGoFiles = append(allGoFiles, pf)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		goFiles[name] = pf
		if len(goFiles) == 1 {
			packageName = pf.Name.Name
			packageNameFile = name
		} else if pf.Name.Name != packageName {
			return nil, &BadPackageError{Err: &build.MultiplePackageError{
				Dir:      innerPath,
				Packages: []string{packageName, pf.Name.Name},
				Files:    []string{packageNameFile, name},
			}}
		}
	}
	if len(goFiles) == 0 {
		// This directory doesn't contain a package, or at least not one
		// that matches this build context.
		return nil, nil
	}

	// The "builtin" package in the standard library is a special case.
	// We want to show documentation for all globals (not just exported ones),
	// and avoid association of consts, vars, and factory functions with types
	// since it's not helpful (see golang.org/issue/6645).
	var noFiltering, noTypeAssociation bool
	if modulePath == stdlib.ModulePath && innerPath == "builtin" {
		noFiltering = true
		noTypeAssociation = true
	}

	// Compute package documentation.
	importPath := path.Join(modulePath, innerPath)
	var m doc.Mode
	if noFiltering {
		m |= doc.AllDecls
	}
	d, err := doc.NewFromFiles(fset, allGoFiles, importPath, m)
	if err != nil {
		return nil, fmt.Errorf("doc.NewFromFiles: %v", err)
	}
	if d.ImportPath != importPath || d.Name != packageName {
		panic(fmt.Errorf("internal error: *doc.Package has an unexpected import path (%q != %q) or package name (%q != %q)", d.ImportPath, importPath, d.Name, packageName))
	}
	if noTypeAssociation {
		for _, t := range d.Types {
			d.Consts, t.Consts = append(d.Consts, t.Consts...), nil
			d.Vars, t.Vars = append(d.Vars, t.Vars...), nil
			d.Funcs, t.Funcs = append(d.Funcs, t.Funcs...), nil
		}
		sort.Slice(d.Funcs, func(i, j int) bool { return d.Funcs[i].Name < d.Funcs[j].Name })
	}

	// Process package imports.
	if len(d.Imports) > maxImportsPerPackage {
		return nil, fmt.Errorf("%d imports found package %q; exceeds limit %d for maxImportsPerPackage", len(d.Imports), importPath, maxImportsPerPackage)
	}

	// Render documentation HTML.
	sourceLinkFunc := func(n ast.Node) string {
		if sourceInfo == nil {
			return ""
		}
		p := fset.Position(n.Pos())
		if p.Line == 0 { // invalid Position
			return ""
		}
		return sourceInfo.LineURL(path.Join(innerPath, p.Filename), p.Line)
	}
	fileLinkFunc := func(filename string) string {
		if sourceInfo == nil {
			return ""
		}
		return sourceInfo.FileURL(path.Join(innerPath, filename))
	}

	docHTML, err := dochtml.Render(ctx, fset, d, dochtml.RenderOptions{
		FileLinkFunc:   fileLinkFunc,
		SourceLinkFunc: sourceLinkFunc,
		ModInfo:        modInfo,
		Limit:          int64(MaxDocumentationHTML),
	})
	if errors.Is(err, dochtml.ErrTooLarge) {
		docHTML = template.MustParseAndExecuteToHTML(docTooLargeReplacement)
	} else if err != nil {
		return nil, fmt.Errorf("dochtml.Render: %v", err)
	}
	if modulePath == stdlib.ModulePath {
		importPath = innerPath
	}
	v1path := internal.V1Path(importPath, modulePath)
	return &internal.LegacyPackage{
		Path:              importPath,
		Name:              packageName,
		Synopsis:          doc.Synopsis(d.Doc),
		V1Path:            v1path,
		Imports:           d.Imports,
		DocumentationHTML: docHTML,
		GOOS:              goos,
		GOARCH:            goarch,
	}, err
}

// matchingFiles returns a map from file names to their contents, read from zipGoFiles.
// It includes only those files that match the build context determined by goos and goarch.
func matchingFiles(goos, goarch string, zipGoFiles []*zip.File) (files map[string][]byte, err error) {
	defer derrors.Wrap(&err, "matchingFiles(%q, %q, zipGoFiles)", goos, goarch)
	// Populate the map with all the zip files.
	files = make(map[string][]byte)
	for _, f := range zipGoFiles {
		_, name := path.Split(f.Name)
		b, err := readZipFile(f, MaxFileSize)
		if err != nil {
			return nil, err
		}
		files[name] = b
	}

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
			return ioutil.NopCloser(bytes.NewReader(files[name])), nil
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

	for name := range files {
		match, err := bctx.MatchFile(".", name) // This will access the file we just added to files map above.
		if err != nil {
			return nil, &BadPackageError{Err: fmt.Errorf(`bctx.MatchFile(".", %q): %w`, name, err)}
		}
		if !match {
			// Excluded by build context.
			delete(files, name)
		}
	}
	return files, nil
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

func allocMeg() int {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int(ms.Alloc / (1024 * 1024))
}

// mib is the number of bytes in a mebibyte (Mi).
const mib = 1024 * 1024

// The largest module zip size we can comfortably process.
// We probably will OOM if we process a module whose zip is larger.
var maxModuleZipSize int64 = math.MaxInt64

func init() {
	m := os.Getenv("GO_DISCOVERY_MAX_MODULE_ZIP_MI")
	if m != "" {
		v, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			log.Errorf(context.Background(), "could not parse GO_DISCOVERY_MAX_MODULE_ZIP_MI value %q", v)
		} else {
			maxModuleZipSize = v * mib
		}
	}
}

var zipLoadShedder = loadShedder{maxSizeInFlight: math.MaxUint64}

func init() {
	ctx := context.Background()
	m := os.Getenv("GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI")
	if m != "" {
		mebis, err := strconv.ParseUint(m, 10, 64)
		if err != nil {
			log.Fatalf(ctx, "could not parse GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI value %q", m)
		} else if mebis == 0 {
			log.Fatalf(ctx, "bad value for GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI: %d. Must be >= 1.", mebis)
		} else {
			log.Infof(ctx, "shedding load over %dMi", mebis)
			zipLoadShedder.maxSizeInFlight = mebis * mib
		}
	}
}

// ZipLoadShedStats returns a snapshot of the current LoadShedStats for zip files.
func ZipLoadShedStats() LoadShedStats {
	return zipLoadShedder.stats()
}
