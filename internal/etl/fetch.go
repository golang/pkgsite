// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

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
	"net/http"
	"os"
	"path"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/dzip"
	"golang.org/x/discovery/internal/etl/dochtml"
	"golang.org/x/discovery/internal/etl/internal/doc"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/xerrors"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
	errReadmeNotFound           = errors.New("module does not contain a README")

	// fetchTimeout bounds the time allowed for fetching a single module.  It is
	// mutable for testing purposes.
	fetchTimeout = 10 * time.Minute

	maxPackagesPerModule = 10000
	maxImportsPerPackage = 1000
)

// appVersionLabel is used to mark the app version at which a module version is
// fetched. It is mutable for testing purposes.
var appVersionLabel = config.AppVersionLabel()

// Indicates that although we have a valid module, some packages could not be processed.
const hasIncompletePackagesCode = 290

// fetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_states table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func fetchAndUpdateState(ctx context.Context, modulePath, version string, client *proxy.Client, db *postgres.DB) (_ int, err error) {
	defer derrors.Wrap(&err, "fetchAndUpdateState(%q, %q)", modulePath, version)

	ctx, span := trace.StartSpan(ctx, "fetchAndUpdateState")
	span.AddAttributes(
		trace.StringAttribute("modulePath", modulePath),
		trace.StringAttribute("version", version))
	defer span.End()
	var (
		code     = http.StatusOK
		fetchErr error
	)
	hasIncompletePackages, fetchErr := fetchAndInsertVersion(ctx, modulePath, version, client, db)
	if fetchErr != nil {
		log.Errorf("Error executing fetch: %v", fetchErr)
		code = derrors.ToHTTPStatus(fetchErr)
	}
	if hasIncompletePackages {
		code = hasIncompletePackagesCode
	}

	
	
	
	if code == http.StatusNotFound || code == http.StatusGone {
		log.Infof("%s@%s: proxy said 404/410, deleting", modulePath, version)
		if err := db.DeleteVersion(ctx, nil, modulePath, version); err != nil {
			log.Error(err)
			return http.StatusInternalServerError, err
		}
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later action.

	// TODO(b/139178863): Split UpsertVersionState into InsertVersionState and UpdateVersionState.
	if err := db.UpsertVersionState(ctx, modulePath, version, appVersionLabel, time.Time{}, code, fetchErr); err != nil {
		log.Error(err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}
	log.Infof("Updated version state for %s@%s: code=%d, hasIncompletePackages=%t err=%v",
		modulePath, version, code, hasIncompletePackages, fetchErr)
	return code, fetchErr
}

// fetchAndInsertVersion fetches the given module version from the module proxy
// or (in the case of the standard library) from the Go repo and writes the
// resulting data to the database.
//
// The given parentCtx is used for tracing, but fetches actually execute in a
// detached context with fixed timeout, so that fetches are allowed to complete
// even for short-lived requests.
func fetchAndInsertVersion(parentCtx context.Context, modulePath, version string, proxyClient *proxy.Client, db *postgres.DB) (hasIncompletePackages bool, err error) {
	defer derrors.Wrap(&err, "fetchAndInsertVersion(%q, %q)", modulePath, version)

	parentSpan := trace.FromContext(parentCtx)
	// A fixed timeout for fetchAndInsertVersion to allow module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	ctx, span := trace.StartSpanWithRemoteParent(ctx, "fetchAndInsertVersion", parentSpan.SpanContext())
	defer span.End()

	v, hasIncompletePackages, err := FetchVersion(ctx, modulePath, version, proxyClient)
	if err != nil {
		return false, err
	}
	log.Infof("Fetched %s@%s", v.ModulePath, v.Version)
	if err = db.InsertVersion(ctx, v); err != nil {
		return false, err
	}
	log.Infof("Inserted version %s@%s", v.ModulePath, v.Version)
	return hasIncompletePackages, nil
}

// FetchVersion queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and processes the contents to return an
// *internal.Version.
func FetchVersion(ctx context.Context, modulePath, version string, proxyClient *proxy.Client) (_ *internal.Version, hasIncompletePackages bool, err error) {
	defer derrors.Wrap(&err, "fetchVersion(%q, %q)", modulePath, version)

	var commitTime time.Time
	var zipReader *zip.Reader
	if modulePath == stdlib.ModulePath {
		zipReader, commitTime, err = stdlib.Zip(version)
		if err != nil {
			return nil, false, err
		}
	} else {
		info, err := proxyClient.GetInfo(ctx, modulePath, version)
		if err != nil {
			return nil, false, err
		}
		version = info.Version
		commitTime = info.Time
		zipReader, err = proxyClient.GetZip(ctx, modulePath, version)
		if err != nil {
			return nil, false, err
		}
	}
	versionType, err := internal.ParseVersionType(version)
	if err != nil {
		return nil, false, xerrors.Errorf("%v: %w", err, derrors.BadModule)
	}

	return processZipFile(ctx, modulePath, versionType, version, commitTime, zipReader)
}

// processZipFile extracts information from the module version zip.
func processZipFile(ctx context.Context, modulePath string, versionType internal.VersionType, version string, commitTime time.Time, zipReader *zip.Reader) (_ *internal.Version, hasIncompletePackages bool, err error) {
	defer derrors.Wrap(&err, "processZipFile(%q, %q)", modulePath, version)

	_, span := trace.StartSpan(ctx, "processing zipFile")
	defer span.End()

	var repoURL string
	sourceInfo, err := source.ModuleInfo(ctx, modulePath, version)
	if err != nil {
		log.Error(err)
	} else {
		repoURL = sourceInfo.RepoURL
	}
	readmeFilePath, readmeContents, err := extractReadmeFromZip(modulePath, version, zipReader)
	if err != nil && err != errReadmeNotFound {
		return nil, false, fmt.Errorf("extractReadmeFromZip(%q, %q, zipReader): %v", modulePath, version, err)
	}
	licenses, err := license.Detect(moduleVersionDir(modulePath, version), zipReader)
	if err != nil {
		log.Error(err)
	}
	packages, hasIncompletePackages, err := extractPackagesFromZip(modulePath, version, zipReader, license.NewMatcher(licenses), sourceInfo)
	if err == errModuleContainsNoPackages {
		return nil, false, xerrors.Errorf("%v: %w", errModuleContainsNoPackages.Error(), derrors.BadModule)
	}
	if err != nil {
		return nil, false, fmt.Errorf("extractPackagesFromZip(%q, %q, zipReader, %v): %v", modulePath, version, licenses, err)
	}
	return &internal.Version{
		VersionInfo: internal.VersionInfo{
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     commitTime,
			ReadmeFilePath: readmeFilePath,
			ReadmeContents: readmeContents,
			VersionType:    versionType,
			RepositoryURL:  repoURL,
		},
		Packages: packages,
		Licenses: licenses,
	}, hasIncompletePackages, nil
}

// moduleVersionDir formats the content subdirectory for the given
// modulePath and version.
func moduleVersionDir(modulePath, version string) string {
	return fmt.Sprintf("%s@%s", modulePath, version)
}

// extractReadmeFromZip returns the file path and contents of the first file
// from r that is a README file. errReadmeNotFound is returned if a README is
// not found.
func extractReadmeFromZip(modulePath, version string, r *zip.Reader) (string, []byte, error) {
	for _, zipFile := range r.File {
		if hasFilename(zipFile.Name, "README") {
			if zipFile.UncompressedSize64 > dzip.MaxFileSize {
				return "", nil, fmt.Errorf("file size %d exceeds max limit %d", zipFile.UncompressedSize64, dzip.MaxFileSize)
			}
			c, err := dzip.ReadZipFile(zipFile)
			if err != nil {
				return "", nil, err
			}
			return strings.TrimPrefix(zipFile.Name, moduleVersionDir(modulePath, version)+"/"), c, nil
		}
	}
	return "", nil, errReadmeNotFound
}

// hasFilename reports whether file is expectedFile or if the base name of file,
// with or without the extension, is equal to expectedFile. It is case
// insensitive. It operates on '/'-separated paths.
func hasFilename(file string, expectedFile string) bool {
	base := path.Base(file)
	return strings.EqualFold(file, expectedFile) ||
		strings.EqualFold(base, expectedFile) ||
		strings.EqualFold(strings.TrimSuffix(base, path.Ext(base)), expectedFile)
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
// It matches against the given licenses to determine the subset of licenses
// that applies to each package.
// The second return value says whether any packages are "incomplete," meaning
// that they contained .go files but couldn't be processed due to current
// limitations of this site. The two limitations are a maximum file size
// (dzip.MaxFileSize), and the particular set of build contexts we consider
// (goEnvs).
func extractPackagesFromZip(modulePath, version string, r *zip.Reader, matcher license.Matcher, sourceInfo *source.Info) (_ []*internal.Package, hasIncompletePackages bool, err error) {

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
		// modulePrefix is the "<module>@<version>/" prefix that all files
		// are expected to have according to the zip archive layout specification
		// at the bottom of https://golang.org/cmd/go/#hdr-Module_proxy_protocol.
		modulePrefix = moduleVersionDir(modulePath, version) + "/"

		// dirs is the set of directories with at least one .go file,
		// to be populated during phase 1 and used during phase 2.
		//
		// The map key is the directory path, with the modulePrefix trimmed.
		// The map value is a slice of all .go files, and no other files.
		dirs = make(map[string][]*zip.File)

		// incompleteDirs tracks directories for which we have incomplete
		// information, due to a problem processing one of the go files contained
		// therein. We use this so that a single unprocessable package does not
		// prevent processing of other packages in the module.
		incompleteDirs = make(map[string]bool)
	)

	// Phase 1.
	// Loop over zip files preemptively and check for problems
	// that can be detected by looking at metadata alone.
	// We'll be looking at file contents starting with phase 2 only,
	// only after we're sure this phase passed without errors.
	for _, f := range r.File {
		if f.Mode().IsDir() {
			return nil, false, fmt.Errorf("expected only files, found directory %q", f.Name)
		}
		if !strings.HasPrefix(f.Name, modulePrefix) {
			return nil, false, fmt.Errorf("expected file to have prefix %q; got = %q", modulePrefix, f.Name)
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
		if f.UncompressedSize64 > dzip.MaxFileSize {
			log.Infof("Unable to process %s: file size %d exceeds max limit %d",
				f.Name, f.UncompressedSize64, dzip.MaxFileSize)
			incompleteDirs[innerPath] = true
			continue
		}
		dirs[innerPath] = append(dirs[innerPath], f)
		if len(dirs) > maxPackagesPerModule {
			return nil, false, fmt.Errorf("%d packages found in %q; exceeds limit %d for maxPackagePerModule", len(dirs), modulePath, maxPackagesPerModule)
		}
	}

	// Phase 2.
	// If we got this far, the file metadata was okay.
	// Start reading the file contents now to extract information
	// about Go packages.
	var pkgs []*internal.Package
	for innerPath, goFiles := range dirs {
		if incompleteDirs[innerPath] {
			// Something went wrong when processing this directory, so we skip.
			log.Infof("Skipping %q because it is incomplete", innerPath)
			continue
		}
		pkg, err := loadPackage(goFiles, innerPath, modulePath, sourceInfo)
		if p := (&BadPackageError{}); xerrors.As(err, &p) {
			// TODO(b/133187024): Record and display this information instead of just skipping.
			log.Infof("Skipping %q because of *BadPackageError: %v\n", path.Join(modulePath, innerPath), err)
			continue
		} else if err != nil {
			return nil, false, fmt.Errorf("unexpected error loading package: %v", err)
		}
		if pkg == nil {
			// No package.
			if len(goFiles) > 0 {
				// There were go files, but no build contexts matched them.
				incompleteDirs[innerPath] = true
			}
			continue
		}
		pkg.Licenses = matcher.Match(innerPath)
		pkgs = append(pkgs, pkg)
	}
	if len(pkgs) == 0 {
		return nil, false, errModuleContainsNoPackages
	}
	return pkgs, len(incompleteDirs) > 0, nil
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
func loadPackage(zipGoFiles []*zip.File, innerPath, modulePath string, sourceInfo *source.Info) (*internal.Package, error) {
	for _, env := range goEnvs {
		pkg, err := loadPackageWithBuildContext(env.GOOS, env.GOARCH, zipGoFiles, innerPath, modulePath, sourceInfo)
		if err != nil {
			return nil, err
		}
		if pkg != nil {
			return pkg, nil
		}
	}
	return nil, nil
}

// loadPackageWithBuildContext loads a Go package made of .go files in zipGoFiles
// using a build context constructed from the given GOOS and GOARCH values.
// modulePath is stdlib.ModulePath for the Go standard library and the module
// path for all other modules. innerPath is the path of the Go package directory
// relative to the module root.
//
// zipGoFiles must contain only .go files that have been verified
// to be of reasonable size.
//
// The returned Package.Licenses field is not populated.
//
// It returns a nil Package if the directory doesn't contain a Go package
// or all .go files have been excluded by constraints.
// A *BadPackageError error is returned if the directory
// contains .go files but do not make up a valid package.
func loadPackageWithBuildContext(goos, goarch string, zipGoFiles []*zip.File, innerPath, modulePath string, sourceInfo *source.Info) (_ *internal.Package, err error) {
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
		testGoFiles     = make(map[string]*ast.File)
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
		if strings.HasSuffix(name, "_test.go") {
			testGoFiles[name] = pf
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
	apkg, _ := ast.NewPackage(fset, goFiles, simpleImporter, nil) // Ignore errors that can happen due to unresolved identifiers.
	for name, f := range testGoFiles {                            // TODO(b/137567588): Improve upstream doc.New API design.
		apkg.Files[name] = f
	}
	importPath := path.Join(modulePath, innerPath)
	var m doc.Mode
	if noFiltering {
		m |= doc.AllDecls
	}
	d := doc.New(apkg, importPath, m)
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
	docHTML, err := dochtml.Render(fset, d, sourceLinkFunc)
	if err != nil {
		return nil, fmt.Errorf("dochtml.Render: %v", err)
	}

	v1path := path.Join(internal.SeriesPathForModule(modulePath), innerPath)
	if modulePath == stdlib.ModulePath {
		importPath = innerPath
		v1path = innerPath
	}
	return &internal.Package{
		Path:              importPath,
		Name:              packageName,
		Synopsis:          doc.Synopsis(d.Doc),
		V1Path:            v1path,
		Imports:           d.Imports,
		DocumentationHTML: docHTML,
		GOOS:              goos,
		GOARCH:            goarch,
	}, nil
}

// matchingFiles returns a map from file names to their contents, read from zipGoFiles.
// It includes only those files that match the build context determined by goos and goarch.
func matchingFiles(goos, goarch string, zipGoFiles []*zip.File) (files map[string][]byte, err error) {
	defer derrors.Wrap(&err, "matchingFiles(%q, %q, zipGoFiles)", goos, goarch)
	// Populate the map with all the zip files.
	files = make(map[string][]byte)
	for _, f := range zipGoFiles {
		_, name := path.Split(f.Name)
		b, err := dzip.ReadZipFile(f)
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
			return nil, &BadPackageError{Err: xerrors.Errorf(`bctx.MatchFile(".", %q): %w`, name, err)}
		}
		if !match {
			// Excluded by build context.
			delete(files, name)
		}
	}
	return files, nil
}

// simpleImporter returns a (dummy) package object named by the last path
// component of the provided package path (as is the convention for packages).
// This is sufficient to resolve package identifiers without doing an actual
// import. It never returns an error.
func simpleImporter(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg == nil {
		// note that strings.LastIndex returns -1 if there is no "/"
		pkg = ast.NewObj(ast.Pkg, path[strings.LastIndex(path, "/")+1:])
		pkg.Data = ast.NewScope(nil) // required by ast.NewPackage for dot-import
		imports[path] = pkg
	}
	return pkg, nil
}
