// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/pkgsite/internal/vuln"
)

const (
	// maxSearchResults is the maximum number of search results to return for a search query.
	maxSearchResults = 1000
	// defaultLimit is the default number of results to return per page for paginated results.
	defaultLimit = 100
)

// ServePackage handles requests for the v1 package metadata endpoint.
func ServePackage(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackage")

	pkgPath := trimPath(r, "/v1/package/")
	if pkgPath == "" {
		return BadRequest("missing package path")
	}

	var params PackageParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	um, err := resolveModulePath(r, ds, pkgPath, params.Module, params.Version)
	if err != nil {
		return err
	}

	fs := internal.WithMain
	if params.Licenses {
		fs |= internal.WithLicenses
	}
	if params.Imports {
		fs |= internal.WithImports
	}
	if params.Doc != "" || params.Examples {
		fs |= internal.WithDocsSource
	}

	bc := internal.BuildContext{GOOS: params.GOOS, GOARCH: params.GOARCH}
	unit, err := ds.GetUnit(r.Context(), um, fs, bc)
	if err != nil {
		return err
	}

	resp, err := unitToPackage(unit, params)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(params.Version))
}

// ServeModule handles requests for the v1 module metadata endpoint.
func ServeModule(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModule")

	modulePath := trimPath(r, "/v1/module/")
	if modulePath == "" {
		return BadRequest("missing module path")
	}

	var params ModuleParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}
	// The served response is cacheDur if and only if the version is.
	cacheDur := versionCacheDur(requestedVersion)

	// For modules, we can use GetUnitMeta on the module path.
	um, err := ds.GetUnitMeta(r.Context(), modulePath, modulePath, requestedVersion)
	if err != nil {
		return err
	}

	resp := Module{
		Path:              um.ModulePath,
		Version:           um.Version,
		IsLatest:          um.Version == um.LatestVersion,
		IsStandardLibrary: stdlib.Contains(um.ModulePath),
		IsRedistributable: um.IsRedistributable,
		HasGoMod:          um.HasGoMod,
	}
	// RepoURL needs to be extracted from source info if available
	if um.SourceInfo != nil {
		resp.RepoURL = um.SourceInfo.RepoURL()
	}

	if !params.Readme && !params.Licenses {
		return serveJSON(w, http.StatusOK, resp, cacheDur)
	}

	fs := internal.MinimalFields
	if params.Readme {
		fs |= internal.WithMain // WithMain includes Readme in GetUnit
	}
	if params.Licenses {
		fs |= internal.WithLicenses
	}
	unit, err := ds.GetUnit(r.Context(), um, fs, internal.BuildContext{})
	if err != nil {
		return serveJSON(w, http.StatusOK, resp, cacheDur)
	}

	if params.Readme && unit.Readme != nil {
		resp.Readme = &Readme{
			Filepath: unit.Readme.Filepath,
			Contents: unit.Readme.Contents,
		}
	}
	if params.Licenses {
		for _, l := range unit.LicenseContents {
			resp.Licenses = append(resp.Licenses, License{
				Types:    l.Metadata.Types,
				FilePath: l.Metadata.FilePath,
				Contents: string(l.Contents),
			})
		}
	}

	return serveJSON(w, http.StatusOK, resp, cacheDur)
}

// ServeModuleVersions handles requests for the v1 module versions endpoint.
func ServeModuleVersions(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModuleVersions")

	path := trimPath(r, "/v1/versions/")
	if path == "" {
		return BadRequest("missing path")
	}

	var params VersionsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	infos, err := ds.GetVersionsForPath(r.Context(), path)
	if err != nil {
		return err
	}
	// If there are no versions for the path, then the module doesn't exist.
	if len(infos) == 0 {
		return fmt.Errorf("module %q: %w", path, derrors.NotFound)
	}

	if params.Filter != "" {
		infos = filter(infos, func(info *internal.ModuleInfo) bool {
			return strings.Contains(info.Version, params.Filter)
		})
	}

	resp, err := paginate(infos, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	// The response is never immutable, because a new version can arrive at any time.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServeModulePackages handles requests for the v1 module packages endpoint.
func ServeModulePackages(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModulePackages")

	modulePath := trimPath(r, "/v1/packages/")
	if modulePath == "" {
		return BadRequest("missing module path")
	}

	var params PackagesParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	metas, err := ds.GetModulePackages(r.Context(), modulePath, requestedVersion)
	if err != nil {
		return err
	}

	if params.Filter != "" {
		metas = filter(metas, func(m *internal.PackageMeta) bool {
			return strings.Contains(m.Path, params.Filter) || strings.Contains(m.Synopsis, params.Filter)
		})
	}
	var results []Package
	for _, m := range metas {
		results = append(results, Package{
			Path:              m.Path,
			ModulePath:        modulePath,
			ModuleVersion:     requestedVersion,
			Synopsis:          m.Synopsis,
			IsStandardLibrary: stdlib.Contains(modulePath),
		})
	}

	resp, err := paginate(results, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(requestedVersion))
}

// ServeSearch handles requests for the v1 search endpoint.
func ServeSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeSearch")

	var params SearchParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	if params.Query == "" {
		return BadRequest("missing query")
	}

	dbresults, err := ds.Search(r.Context(), params.Query, internal.SearchOptions{
		MaxResults:    maxSearchResults,
		SearchSymbols: params.Symbol != "",
		SymbolFilter:  params.Symbol,
	})
	if err != nil {
		return err
	}

	if params.Filter != "" {
		dbresults = filter(dbresults, func(r *internal.SearchResult) bool {
			return strings.Contains(r.Synopsis, params.Filter) || strings.Contains(r.PackagePath, params.Filter)
		})
	}
	var results []SearchResult
	for _, r := range dbresults {
		results = append(results, SearchResult{
			PackagePath: r.PackagePath,
			ModulePath:  r.ModulePath,
			Version:     r.Version,
			Synopsis:    r.Synopsis,
		})
	}

	resp, err := paginate(results, params.ListParams, defaultLimit)
	if err != nil {
		return fmt.Errorf("%w: %s", derrors.InvalidArgument, err.Error())
	}

	// Search results are never immutable, because new modules are always being added.
	// NOTE: the default cache freshness is set to 1 hour (see serveJSON). This seems
	// like a reasonable time to cache a search, but be aware of complaints
	// about stale search results.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServePackageSymbols handles requests for the v1 package symbols endpoint.
func ServePackageSymbols(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageSymbols")

	pkgPath := trimPath(r, "/v1/symbols/")
	if pkgPath == "" {
		return BadRequest("missing package path")
	}

	var params SymbolsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	um, err := resolveModulePath(r, ds, pkgPath, params.Module, params.Version)
	if err != nil {
		return err
	}

	bc := internal.BuildContext{GOOS: params.GOOS, GOARCH: params.GOARCH}
	syms, err := ds.GetSymbols(r.Context(), pkgPath, um.ModulePath, um.Version, bc)
	if err != nil {
		return err
	}

	if params.Filter != "" {
		syms = filter(syms, func(s *internal.Symbol) bool {
			return strings.Contains(s.Name, params.Filter) || strings.Contains(s.Synopsis, params.Filter)
		})
	}
	var items []Symbol
	for _, s := range syms {
		items = append(items, Symbol{
			ModulePath: um.ModulePath,
			Version:    um.Version,
			Name:       s.Name,
			Kind:       string(s.Kind),
			Synopsis:   s.Synopsis,
			Parent:     s.ParentName,
		})
	}

	resp, err := paginate(items, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(params.Version))
}

// ServePackageImportedBy handles requests for the v1 package imported-by endpoint.
func ServePackageImportedBy(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageImportedBy")

	pkgPath := trimPath(r, "/v1/imported-by/")
	if pkgPath == "" {
		return BadRequest("missing package path")
	}

	var params ImportedByParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	um, err := resolveModulePath(r, ds, pkgPath, params.Module, requestedVersion)
	if err != nil {
		return err
	}
	modulePath := um.ModulePath

	importedBy, err := ds.GetImportedBy(r.Context(), pkgPath, modulePath, 1000)
	if err != nil {
		return err
	}

	count, err := ds.GetImportedByCount(r.Context(), pkgPath, modulePath)
	if err != nil {
		return err
	}

	if params.Filter != "" {
		importedBy = filter(importedBy, func(p string) bool {
			return strings.Contains(p, params.Filter)
		})
	}

	paged, err := paginate(importedBy, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	resp := PackageImportedBy{
		ModulePath: modulePath,
		Version:    requestedVersion,
		ImportedBy: PaginatedResponse[string]{
			Items:         paged.Items,
			Total:         count,
			NextPageToken: paged.NextPageToken,
		},
	}

	// The imported-by list is not immutable, because new modules are always being added.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServeVulnerabilities handles requests for the v1 module vulnerabilities endpoint.
func ServeVulnerabilities(vc *vuln.Client) func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	return func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
		defer derrors.Wrap(&err, "ServeVulnerabilities")

		modulePath := trimPath(r, "/v1/vulns/")
		if modulePath == "" {
			return BadRequest("missing module path")
		}

		var params VulnParams
		if err := ParseParams(r.URL.Query(), &params); err != nil {
			return err
		}

		if vc == nil {
			return InternalServerError("vulnerability client is nil")
		}

		requestedVersion := params.Version
		if requestedVersion == "" {
			requestedVersion = version.Latest
		}

		// Use VulnsForPackage from internal/vuln to get vulnerabilities for the module.
		// Passing an empty packagePath gets all vulns for the module.
		vulns := vuln.VulnsForPackage(r.Context(), modulePath, requestedVersion, "", vc)

		if params.Filter != "" {
			vulns = filter(vulns, func(v vuln.Vuln) bool {
				return strings.Contains(v.ID, params.Filter) || strings.Contains(v.Details, params.Filter)
			})
		}
		var items []Vulnerability
		for _, v := range vulns {
			items = append(items, Vulnerability{
				ID:      v.ID,
				Details: v.Details,
			})
		}

		resp, err := paginate(items, params.ListParams, defaultLimit)
		if err != nil {
			return err
		}

		return serveJSON(w, http.StatusOK, resp, versionCacheDur(requestedVersion))
	}
}

func trimPath(r *http.Request, prefix string) string {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	return strings.Trim(path, "/")
}

// resolveModulePath determines the correct module path for a given package path and version.
// If the module path is not provided, it searches through potential candidate module paths
// derived from the package path.
//
// Resolution logic:
//  1. Use internal.CandidateModulePaths(pkgPath) to get potential candidates (ordered longest first).
//  2. Fetch UnitMeta for each candidate that exists in the data source.
//  3. Check if um.ModulePath == mp (where mp is the candidate module path). If not, ignore it
//     (this handles the case where GetUnitMeta falls back to another module when the requested
//     module does not exist).
//  4. Filter candidates by eliminating those that are deprecated or retracted.
//  5. If exactly one candidate remains after filtering, return it (HTTP 200).
//  6. If multiple candidates remain, return HTTP 400 with the list of candidates (ambiguity).
//  7. If all candidates are eliminated (e.g., all are deprecated or retracted), fall back to
//     the longest matching candidate among those that exist (HTTP 200).
func resolveModulePath(r *http.Request, ds internal.DataSource, pkgPath, modulePath, requestedVersion string) (*internal.UnitMeta, error) {
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}
	if modulePath != "" {
		um, err := ds.GetUnitMeta(r.Context(), pkgPath, modulePath, requestedVersion)
		if err != nil {
			return nil, err
		}
		return um, nil
	}

	candidates := internal.CandidateModulePaths(pkgPath)
	var validCandidates []*internal.UnitMeta
	for _, mp := range candidates {
		if um, err := ds.GetUnitMeta(r.Context(), pkgPath, mp, requestedVersion); err == nil {
			// Critical check: ensure the DB actually found the candidate module we requested.
			// GetUnitMeta falls back to the best match if the requested module doesn't exist,
			// which could lead to false positives (e.g. google.golang.org matching because it
			// falls back to google.golang.org/adk/agent).
			if um.ModulePath == mp {
				validCandidates = append(validCandidates, um)
			}
		} else if !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
	}

	if len(validCandidates) == 0 {
		return nil, derrors.NotFound
	}

	// Filter candidates based on signals (deprecation, retraction).
	goodCandidates := slices.Clone(validCandidates)
	goodCandidates = slices.DeleteFunc(goodCandidates, func(um *internal.UnitMeta) bool {
		return um.Deprecated || um.Retracted
	})

	switch len(goodCandidates) {
	case 1:
		return goodCandidates[0], nil
	case 0:
		// If all candidates are deprecated or retracted, fall back to the longest match.
		// Since candidates are ordered longest first, validCandidates[0] is the longest match.
		return validCandidates[0], nil
	default:
		return nil, &Error{
			Code:       http.StatusBadRequest,
			Message:    "ambiguous package path",
			Candidates: makeCandidates(goodCandidates),
		}
	}
}

func makeCandidates(ums []*internal.UnitMeta) []Candidate {
	var r []Candidate
	for _, um := range ums {
		r = append(r, Candidate{
			ModulePath:  um.ModulePath,
			PackagePath: um.Path,
		})
	}
	return r
}

// Values for the Cache-Control header.
// Compare with the TTLs for pkgsite's own cache, in internal/frontend/server.go
// (look for symbols ending in "TTL").
// Those values are shorter to manage our cache's memory, but the job of
// Cache-Control is to reduce network traffic; downstream caches can manage
// their own memory.
const (
	// Immutable pages can theoretically, be cached indefinitely,
	// but have them time out so that excluded modules don't
	// live in caches forever.
	longCacheDur = 3 * time.Hour
	// The information on some pages can change relatively quickly.
	shortCacheDur = 1 * time.Hour
	// Errors should not be cached.
	noCache = time.Duration(0)
)

func serveJSON(w http.ResponseWriter, status int, data any, cacheDur time.Duration) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	var ccHeader string
	if cacheDur == 0 {
		ccHeader = "no-store"
	} else {
		ccHeader = fmt.Sprintf("public, max-age=%d", int(cacheDur.Seconds()))
	}
	w.Header().Set("Cache-Control", ccHeader)
	w.WriteHeader(status)
	_, err := w.Write(buf.Bytes())
	return err
}

func ServeError(w http.ResponseWriter, r *http.Request, err error) error {
	var aerr *Error
	if !errors.As(err, &aerr) {
		status := derrors.ToStatus(err)
		aerr = &Error{
			Code:    status,
			Message: strings.ToLower(http.StatusText(status)),
			err:     err,
		}
	}
	log.Errorf(r.Context(), "API error %d: %v", aerr.Code, aerr)
	return serveJSON(w, aerr.Code, aerr, noCache)
}

// paginate returns a paginated response for the given list of items and pagination parameters.
// It uses offset-based pagination with a token that encodes the offset.
// The default limit is used if the provided limit is non-positive.
func paginate[T any](all []T, lp ListParams, defaultLimit int) (PaginatedResponse[T], error) {
	limit := lp.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	offset := 0
	if lp.Token != "" {
		var err error
		offset, err = strconv.Atoi(lp.Token)
		if err != nil || offset < 0 {
			return PaginatedResponse[T]{}, fmt.Errorf("%w: invalid token", derrors.InvalidArgument)
		}
	}

	if offset > len(all) {
		offset = len(all)
	}
	end := min(offset+limit, len(all))

	var nextToken string
	if end < len(all) {
		nextToken = strconv.Itoa(end)
	}

	return PaginatedResponse[T]{
		Items:         all[offset:end],
		Total:         len(all),
		NextPageToken: nextToken,
	}, nil
}

// unitToPackage processes unit documentation into a Package struct.
func unitToPackage(unit *internal.Unit, params PackageParams) (*Package, error) {
	if params.Examples && params.Doc == "" {
		return nil, BadRequest("examples require doc format to be specified")
	}

	// Although unit.Documentation is a slice, it will
	// have at most one item, the documentation matching
	// the build context.
	synopsis := ""
	var docs string
	goos := params.GOOS
	goarch := params.GOARCH
	if len(unit.Documentation) > 0 {
		d := unit.Documentation[0]
		synopsis = d.Synopsis
		// Return the more precise GOOS/GOARCH.
		// If the user didn't provide them, use the unit's.
		// If the user did, assume what they provided is at
		// least as specific as the unit's, and use it.
		if goos == "" {
			goos = d.GOOS
		}
		if goarch == "" {
			goarch = d.GOARCH
		}
		if params.Doc != "" {
			var err error
			docs, err = renderDocumentation(unit, d, params.Doc, params.Examples)
			if err != nil {
				return nil, err
			}
		}
	}

	var licenses []License
	for _, l := range unit.LicenseContents {
		licenses = append(licenses, License{
			Types:    l.Metadata.Types,
			FilePath: l.Metadata.FilePath,
			Contents: string(l.Contents),
		})
	}

	return &Package{
		Path:              unit.Path,
		ModulePath:        unit.ModulePath,
		ModuleVersion:     unit.Version,
		Synopsis:          synopsis,
		IsStandardLibrary: stdlib.Contains(unit.ModulePath),
		IsLatest:          unit.Version == unit.LatestVersion,
		GOOS:              goos,
		GOARCH:            goarch,
		Docs:              docs,
		Imports:           unit.Imports,
		Licenses:          licenses,
	}, nil
}

// renderDocumentation renders the provided unit into the specified format.
func renderDocumentation(unit *internal.Unit, d *internal.Documentation, format string, examples bool) (string, error) {
	// d.Source is an encoded AST. Decode it, then use
	// go/doc (not pkgsite's renderer) to generate the
	// result.
	gpkg, err := godoc.DecodePackage(d.Source)
	if err != nil {
		return "", fmt.Errorf("renderDocumentation: %w", err)
	}
	innerPath := internal.Suffix(unit.Path, unit.ModulePath)
	modInfo := &godoc.ModuleInfo{ModulePath: unit.ModulePath, ResolvedVersion: unit.Version}
	dpkg, err := gpkg.DocPackage(innerPath, modInfo)
	if err != nil {
		return "", err
	}
	var r renderer
	var sb strings.Builder
	switch format {
	case "text":
		r = newTextRenderer(gpkg.Fset, &sb)
	case "md", "markdown":
		r = newMarkdownRenderer(gpkg.Fset, &sb)
	case "html":
		r = newHTMLRenderer(gpkg.Fset, &sb)
	default:
		return "", BadRequest("bad doc format: need one of 'text', 'md', 'markdown' or 'html'")
	}
	if err := renderDoc(dpkg, r, examples); err != nil {
		return "", fmt.Errorf("renderDoc: %w", err)
	}
	return sb.String(), nil
}

// versionCacheDur returns the duration used in the Cache-Control header
// appropriate for the given module version.
func versionCacheDur(v string) time.Duration {
	immutable := !(v == "" || v == version.Latest || internal.DefaultBranches[v] || stdlib.SupportedBranches[v])
	if immutable {
		return longCacheDur
	}
	return shortCacheDur
}

// filter returns a new slice containing all elements in list for which pred is true.
func filter[T any](list []T, pred func(T) bool) []T {
	var out []T
	for _, e := range list {
		if pred(e) {
			out = append(out, e)
		}
	}
	return out
}
