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
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/dzip"
	"golang.org/x/discovery/internal/etl/dochtml"
	"golang.org/x/discovery/internal/etl/internal/doc"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/net/context/ctxhttp"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
	errReadmeNotFound           = errors.New("module does not contain a README")

	// fetchTimeout bounds the time allowed for fetching a single module.  It is
	// mutable for testing purposes.
	fetchTimeout = 5 * time.Minute

	maxPackagesPerModule = 10000
	maxImportsPerPackage = 1000

	// vcsClient is used to make HTTP requests to the url where a module is
	// hosted to determine the repository url. It is mutable for testing
	// purposes.
	vcsClient = &http.Client{
		Timeout:   time.Second * 30,
		Transport: &ochttp.Transport{},
	}
)

// appVersionLabel is used to mark the app version at which a module version is
// fetched. It is mutable for testing purposes.
var appVersionLabel = config.AppVersionLabel()

// fetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_state_table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func fetchAndUpdateState(ctx context.Context, modulePath, version string, client *proxy.Client, db *postgres.DB) (int, error) {
	ctx, span := trace.StartSpan(ctx, "fetchAndUpdateState")
	span.AddAttributes(
		trace.StringAttribute("modulePath", modulePath),
		trace.StringAttribute("version", version))
	defer span.End()
	var (
		code     = http.StatusOK
		fetchErr error
	)
	if fetchErr = fetchAndInsertVersion(ctx, modulePath, version, client, db); fetchErr != nil {
		log.Printf("Error executing fetch: %v", fetchErr)
		switch {
		case derrors.IsInvalidArgument(fetchErr):
			code = http.StatusBadRequest
		case derrors.IsNotFound(fetchErr):
			code = http.StatusNotFound
		case derrors.IsNotAcceptable(fetchErr):
			code = http.StatusNotAcceptable
		default:
			code = http.StatusInternalServerError
		}
	}

	if err := db.UpsertVersionState(ctx, modulePath, version, appVersionLabel, time.Time{}, code, fetchErr); err != nil {
		log.Printf("db.UpsertVersionState(ctx, %q, %q, %q, %q, %v): %q", modulePath, version, config.AppVersionLabel(), code, fetchErr, err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}

	return code, fetchErr
}

// fetchAndInsertVersion downloads the given module version from the module proxy, processes
// the contents, and writes the data to the database. The fetch service will:
// (1) Get the repository url for the module
// (2) Get the version commit time from the proxy
// (3) Download the version zip from the proxy
// (4) Process the contents (series path, readme, licenses, packages, documentation)
// (5) Write the data to the discovery database
//
// The given parentCtx is used for tracing, but fetches actually execute in a
// detached context with fixed timeout, so that fetches are allowed to complete
// even for short-lived requests.
func fetchAndInsertVersion(parentCtx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, db *postgres.DB) (err error) {
	defer func() {
		if e := recover(); e != nil {
			// The package processing code performs some sanity checks along the way.
			// None of the panics should occur, but if they do, we want to log them and
			// be able to find them. So, convert internal panics to internal errors here.
			err = fmt.Errorf("internal panic: %v\n\n%s", e, debug.Stack())
		}
	}()

	parentSpan := trace.FromContext(parentCtx)
	// Unlike other actions (which use a Timeout middleware), we set a fixed
	// timeout for fetchAndInsertVersion.  This allows module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	ctx, span := trace.StartSpanWithRemoteParent(ctx, "fetchAndInsertVersion", parentSpan.SpanContext())
	defer span.End()

	repositoryURL, err := fetchRepositoryURL(ctx, modulePath)
	if err != nil {
		log.Printf("fetchRepositoryURL(ctx, %q): %v", modulePath, err)
	}

	info, err := proxyClient.GetInfo(ctx, modulePath, requestedVersion)
	if err != nil {
		// Since this is our first client request, we wrap it to preserve error
		// semantics: if info is not found, then we return NotFound.
		return derrors.Wrap(err, "proxyClient.GetInfo(%q, %q)", modulePath, requestedVersion)
	}
	versionType, err := ParseVersionType(info.Version)
	if err != nil {
		return derrors.NotAcceptable("parseVersion(%q): %v", info.Version, err)
	}
	zipReader, err := proxyClient.GetZip(ctx, modulePath, requestedVersion)
	if err != nil {
		return derrors.Wrap(err, "proxyClient.GetZip(%q, %q)", modulePath, requestedVersion)
	}

	// Module processing is wrapped in an inline func to facilitate tracing.
	var (
		v        *internal.Version
		licenses []*license.License
	)
	if err := func() error {
		_, span := trace.StartSpan(ctx, "processing zipFile")
		defer span.End()
		readmeFilePath, readmeContents, err := extractReadmeFromZip(modulePath, info.Version, zipReader)
		if err != nil && err != errReadmeNotFound {
			return fmt.Errorf("extractReadmeFromZip(%q, %q, zipReader): %v", modulePath, info.Version, err)
		}
		licenses, err = license.Detect(moduleVersionDir(modulePath, info.Version), zipReader)
		if err != nil {
			log.Printf("Error detecting licenses for %v@%v: %v", modulePath, info.Version, err)
		}
		span.Annotate([]trace.Attribute{trace.Int64Attribute("licenseCt", int64(len(licenses)))}, "detected licenses")
		packages, err := extractPackagesFromZip(modulePath, info.Version, zipReader, license.NewMatcher(licenses))
		if err == errModuleContainsNoPackages {
			return derrors.NotAcceptable(errModuleContainsNoPackages.Error())
		}
		if err != nil {
			return fmt.Errorf("extractPackagesFromZip(%q, %q, zipReader, %v): %v", modulePath, info.Version, licenses, err)
		}
		span.Annotate([]trace.Attribute{trace.Int64Attribute("packageCt", int64(len(packages)))}, "extracted packages")

		v = &internal.Version{
			VersionInfo: internal.VersionInfo{
				ModulePath:     modulePath,
				Version:        info.Version,
				CommitTime:     info.Time,
				ReadmeFilePath: readmeFilePath,
				ReadmeContents: readmeContents,
				VersionType:    versionType,
				RepositoryURL:  repositoryURL,
			},
			Packages: packages,
		}
		return nil
	}(); err != nil {
		return err
	}

	if err = db.InsertVersion(ctx, v, licenses); err != nil {
		return fmt.Errorf("db.InsertVersion for %q %q: %v", modulePath, info.Version, err)
	}
	span.Annotate(nil, "inserted version")
	if err = db.InsertDocuments(ctx, v); err != nil {
		return fmt.Errorf("db.InsertDocuments for %q %q: %v", modulePath, info.Version, err)
	}
	span.Annotate(nil, "inserted documents")

	log.Printf("Downloaded: %s@%s", modulePath, info.Version)
	return nil
}

// moduleVersionDir formats the content subdirectory for the given
// modulePath and version.
func moduleVersionDir(modulePath, version string) string {
	return fmt.Sprintf("%s@%s", modulePath, version)
}

var pseudoVersionRE = regexp.MustCompile(`^v[0-9]+\.(0\.0-|\d+\.\d+-([^+]*\.)?0\.)\d{14}-[A-Za-z0-9]+(\+incompatible)?$`)

// isPseudoVersion reports whether a valid version v is a pseudo-version.
// Modified from src/cmd/go/internal/modfetch.
func isPseudoVersion(v string) bool {
	return strings.Count(v, "-") >= 2 && pseudoVersionRE.MatchString(v)
}

// ParseVersionType returns the VersionType of a given a version.
func ParseVersionType(version string) (internal.VersionType, error) {
	if !semver.IsValid(version) {
		return "", fmt.Errorf("semver.IsValid(%q): false", version)
	}

	switch {
	case isPseudoVersion(version):
		return internal.VersionTypePseudo, nil
	case semver.Prerelease(version) != "":
		return internal.VersionTypePrerelease, nil
	default:
		return internal.VersionTypeRelease, nil
	}
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
				return "", nil, fmt.Errorf("ReadZipFile(%q): %v", zipFile.Name, err)
			}
			return strings.TrimPrefix(zipFile.Name, moduleVersionDir(modulePath, version)+"/"), c, nil
		}
	}
	return "", nil, errReadmeNotFound
}

// hasFilename checks if file is expectedFile or if the name of file, without
// the base, is equal to expectedFile. It is case insensitive.
// It operates on '/'-separated paths.
func hasFilename(file string, expectedFile string) bool {
	base := path.Base(file)
	return strings.EqualFold(file, expectedFile) ||
		strings.EqualFold(base, expectedFile) ||
		strings.EqualFold(strings.TrimSuffix(base, path.Ext(base)), expectedFile)
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
// It matches against the given licenses to determine the subset of licenses
// that applies to each package.
func extractPackagesFromZip(modulePath, version string, r *zip.Reader, matcher license.Matcher) ([]*internal.Package, error) {

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
	// only we're sure this phase passed without errors.
	for _, f := range r.File {
		if f.Mode().IsDir() {
			return nil, fmt.Errorf("expected only files, found directory %q", f.Name)
		}
		if !strings.HasPrefix(f.Name, modulePrefix) {
			return nil, fmt.Errorf("expected file to have prefix %q; got = %q", modulePrefix, f.Name)
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
			log.Printf("Unable to process %s: file size %d exceeds max limit %d",
				f.Name, f.UncompressedSize64, dzip.MaxFileSize)
			incompleteDirs[innerPath] = true
			continue
		}
		dirs[innerPath] = append(dirs[innerPath], f)
		if len(dirs) > maxPackagesPerModule {
			return nil, fmt.Errorf("%d packages found in %q; exceeds limit %d for maxPackagePerModule", len(dirs), modulePath, maxPackagesPerModule)
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
			log.Printf("Skipping %q because it is incomplete", innerPath)
			continue
		}
		pkg, err := loadPackage(goFiles, innerPath, modulePath)
		if _, ok := err.(*BadPackageError); ok {
			// TODO(b/133187024): Record and display this information instead of just skipping.
			log.Printf("Skipping %q because of *BadPackageError: %v\n", path.Join(modulePath, innerPath), err)
			continue
		} else if err != nil {
			return nil, fmt.Errorf("unexpected error loading package: %v", err)
		}
		if pkg == nil {
			// No package.
			continue
		}
		pkg.Licenses = matcher.Match(innerPath)
		pkgs = append(pkgs, pkg)
	}
	if len(pkgs) == 0 {
		return nil, errModuleContainsNoPackages
	}
	return pkgs, nil
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

// loadPackage loads a Go package with import path importPath
// from zipGoFiles using the default build context.
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
func loadPackage(zipGoFiles []*zip.File, innerPath, modulePath string) (*internal.Package, error) {
	var (
		// files is a map of file names to their contents.
		//
		// The logic to access it needs to stay in sync across the
		// matchFile, joinPath, and openFile functions below.
		// See the comment inside matchFile for details on how it's used.
		files = make(map[string][]byte)

		// matchFile reports whether the file with the given name and content
		// matches the build context bctx. name must be just the file name, not
		// a file path that includes directory names.
		//
		// The JoinPath and OpenFile fields of bctx must be set to the joinPath
		// and openFile functions below.
		matchFile = func(bctx *build.Context, name string, src []byte) (match bool, err error) {
			// bctx.MatchFile will use bctx.JoinPath to join its first and second parameters,
			// and then use the joined result as the name parameter its call to bctx.OpenFile.
			//
			// Since we control the bctx.OpenFile implementation and have configured it to read
			// from the files map, we need to populate the files map accordingly just before
			// calling bctx.MatchFile.
			//
			// The "." path we're joining with name is arbitrary, it just needs to stay in sync
			// across the calls that populate and access the files map.
			//
			files[bctx.JoinPath(".", name)] = src
			return bctx.MatchFile(".", name) // This will access the file we just added to files map above.
		}

		joinPath = path.Join
		openFile = func(name string) (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(files[name])), nil
		}
	)

	// bctx is the build context. It's used to make decisions about which
	// of the .go files are included or excluded by build constraints.
	bctx := &build.Context{
		GOOS:        "linux",
		GOARCH:      "amd64",
		CgoEnabled:  true,
		Compiler:    build.Default.Compiler,
		ReleaseTags: build.Default.ReleaseTags,

		JoinPath: joinPath,
		OpenFile: openFile,

		// If left nil, the default implementation of these reads from disk,
		// which we do not want. None of these functions should be used
		// inside loadPackage; it would be an internal error if they are.
		// Set them to non-nil values to catch if that happens.
		SplitPathList: func(string) []string { panic("internal error: unexpected call to SplitPathList") },
		IsAbsPath:     func(string) bool { panic("internal error: unexpected call to IsAbsPath") },
		IsDir:         func(string) bool { panic("internal error: unexpected call to IsDir") },
		HasSubdir:     func(string, string) (string, bool) { panic("internal error: unexpected call to HasSubdir") },
		ReadDir:       func(string) ([]os.FileInfo, error) { panic("internal error: unexpected call to ReadDir") },
	}

	// Parse .go files and add them to the goFiles slice.
	// Build constraints are taken into account, and files
	// that don't match are skipped.
	var (
		fset            = token.NewFileSet()
		goFiles         = make(map[string]*ast.File)
		testGoFiles     = make(map[string]*ast.File)
		packageName     string
		packageNameFile string // Name of file where packageName came from.
	)
	for _, f := range zipGoFiles {
		_, name := path.Split(f.Name)
		b, err := dzip.ReadZipFile(f)
		if err != nil {
			return nil, err
		}
		match, err := matchFile(bctx, name, b)
		if err != nil {
			return nil, err
		} else if !match {
			// Excluded by build context.
			continue
		}
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
		// This directory doesn't contain a package.
		// TODO(b/132621615): or does but all .go files excluded by constraints; tell apart
		return nil, nil
	}

	// Compute package documentation.
	apkg, _ := ast.NewPackage(fset, goFiles, simpleImporter, nil) // Ignore errors that can happen due to unresolved identifiers.
	for name, f := range testGoFiles {                            // TODO(b/137567588): Improve upstream doc.New API design.
		apkg.Files[name] = f
	}
	importPath := path.Join(modulePath, innerPath)
	d := doc.New(apkg, importPath, 0)
	if d.ImportPath != importPath || d.Name != packageName {
		panic(fmt.Errorf("internal error: *doc.Package has an unexpected import path (%q != %q) or package name (%q != %q)", d.ImportPath, importPath, d.Name, packageName))
	}

	// Process package imports.
	if len(d.Imports) > maxImportsPerPackage {
		return nil, fmt.Errorf("%d imports found package %q; exceeds limit %d for maxImportsPerPackage", len(d.Imports), importPath, maxImportsPerPackage)
	}

	// Render documentation HTML.
	docHTML, err := dochtml.Render(fset, d)
	if err != nil {
		return nil, fmt.Errorf("dochtml.Render: %v", err)
	}

	v1path := path.Join(internal.SeriesPathForModule(modulePath), innerPath)
	if modulePath == "std" {
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
	}, nil
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

// fetchRepositoryURL returns the repositoryURL for the modulePath if the a GET
// request to the expected repositoryURL returns a 200-series status code.
func fetchRepositoryURL(ctx context.Context, modulePath string) (string, error) {
	repositoryURL, err := modulePathToRepoURL(modulePath)
	if repositoryURL == "" {
		return "", fmt.Errorf("modulePathToRepoURL(%q): %v", modulePath, err)
	}

	r, err := ctxhttp.Get(ctx, vcsClient, repositoryURL)
	if err != nil {
		return "", fmt.Errorf("ctxhttp.Get(ctx, client, %q): %v", modulePath, err)
	}
	if err := derrors.StatusError(r.StatusCode, "ctxhttp.Get(ctx, client, %q)", repositoryURL); err != nil {
		return "", err
	}
	return repositoryURL, nil
}

// acceptedVCSHosts is the list of hosts for which we will try to detect a
// repositoryURL for a given modulePath with that hostname.
var acceptedVCSHosts = map[string]bool{
	"bitbucket.org": true,
	"github.com":    true,
	"golang.org":    true,
}

const goRepositoryURLPrefix = "https://github.com/golang"

// modulePathToRepoURL returns the expected repositoryURL for the
// modulePath. It returns an expected repositoryURL if the modulePath is
// (1) in the acceptedVCSHosts (2) has the prefix golang.org/x or,
// (3) internal.IsStandardLibraryModule(modulePath) returns true. Otherwise,
// the empty string is returned.  It is not guaranteed that the repository url
// returned is a valid url, and this is validated using fetchRepositoryURL.
func modulePathToRepoURL(modulePath string) (string, error) {
	if internal.IsStandardLibraryModule(modulePath) {
		return goRepositoryURLPrefix + "/go", nil
	}

	pathParts := strings.Split(modulePath, "/")
	if ok := acceptedVCSHosts[pathParts[0]]; !ok {
		// If the host (first element of the modulePath) is included in the
		// acceptedVCSHosts, return the empty string.
		return "", fmt.Errorf("repository url could not be determined for %q", modulePath)
	}
	if len(pathParts) < 3 {
		// golang.org/x, github.com and bitbucket.org expect the modulePath to
		// have 3 parts exactly when delimited by a "/". If the modulePath has
		// less than 3 parts, it will not be a valid URL for these hosts, so
		// return the empty string.  Otherwise, use the first three elements
		// for the repository URL. For example, for the module
		// "github.com/hashicorp/vault/api", the repository URL would
		// "github.com/hashicorp/vault".
		return "", fmt.Errorf("expected module with host %q to have at least 3 elements in the path: %q", pathParts[0], modulePath)
	}

	if strings.HasPrefix(modulePath, "golang.org/x/") {
		return goRepositoryURLPrefix + "/" + pathParts[2], nil
	}
	return fmt.Sprintf("https://%s", strings.Join(pathParts[0:3], "/")), nil
}
