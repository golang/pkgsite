// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Comments beginning with "api:" are read by RouteInfos.
// They should not be removed.
// If a new route is added, provide all the "api:" comments for it.

package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"maps"
	"net/http"
	"reflect"
	"slices"
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
	// defaultSearchLimit is the default number of results to return per page for search.
	defaultSearchLimit = 25
	// maxLimit is the maximum allowed limit for paginated results.
	maxLimit = 1000
	// defaultLimit is the default number of results to return per page for paginated results.
	defaultLimit = 100
)

// OpenAPISpec contains the raw bytes of the OpenAPI 3.0 specification for the API.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

// ServePackage handles requests for the v1beta package metadata endpoint.
// api:route /v1beta/package/{path}
// api:desc Information about the package at {path}.
// api:example /v1beta/package/golang.org/x/time/rate
func ServePackage(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackage")

	// api:pathparam path Package path.
	pkgPath := trimPath(r, "/v1beta/package/")
	if pkgPath == "" {
		return BadRequest("missing package path",
			"the package path must be provided after '/package/'")
	}

	// api:params PackageParams
	var params PackageParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	if params.Examples && params.Doc == "" {
		return BadRequest("examples require doc format to be specified")
	}
	switch params.Doc {
	// renderDocumentation needs to be updated when the doc set changes.
	case "", "text", "md", "markdown", "html":
	default:
		return BadRequest("bad doc format: need one of 'text', 'md', 'markdown' or 'html'")
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

	// api:response Package
	resp, err := unitToPackage(unit, params)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(params.Version))
}

// ServeModule handles requests for the v1beta module metadata endpoint.
// api:route /v1beta/module/{path}
// api:desc Information about the module at {path}.
// api:example /v1beta/module/golang.org/x/time
func ServeModule(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModule")

	// api:pathparam path Module path.
	modulePath := trimPath(r, "/v1beta/module/")
	if modulePath == "" {
		return BadRequest("missing module path",
			"the module path must be provided after '/module/'")
	}

	// api:params ModuleParams
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
	um, err := ds.GetUnitMeta(r.Context(), modulePath, internal.UnknownModulePath, requestedVersion)
	if err != nil {
		return err
	}

	if err := checkModulePath(modulePath, um.ModulePath); err != nil {
		return err
	}

	// api:response Module
	resp := Module{
		Path:              um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
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

// ServeModuleVersions handles requests for the v1beta module versions endpoint.
// api:route /v1beta/versions/{path}
// api:desc All versions of the module at {path}, including all major versions.
// api:desc Versions are listed in descending order, with incompatible versions last.
// api:desc Only tagged versions are returned, unless the pseudo query parameter is true.
// api:desc In addition, only results that match the filter query parameter are returned.
// api:desc The total in the response is -1 to indicate that the total number of results is unknown,
// api:desc unless all results fit on a single page.
// api:example /v1beta/versions/golang.org/x/time?limit=3
func ServeModuleVersions(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModuleVersions")

	// api:pathparam path Module path.
	path := trimPath(r, "/v1beta/versions/")
	if path == "" {
		return BadRequest("missing module path",
			"the module path must be provided after '/versions/'")
	}

	// api:params VersionsParams
	var params VersionsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}
	um, err := ds.GetUnitMeta(r.Context(), path, internal.UnknownModulePath, version.Latest)
	if err != nil {
		return fmt.Errorf("module %q: %w", path, err)
	}
	// TODO: generalize this endpoint to packages.
	// That is how the DB query works.
	if err := checkModulePath(path, um.ModulePath); err != nil {
		return err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	start := ""
	if params.Token != "" {
		var err error
		start, err = decodeStringPageToken(params.Token)
		if err != nil {
			return BadRequest(fmt.Sprintf("invalid next-page token: %v", err), "try again from the beginning, with no token")
		}
	}

	// Determine version types to fetch: either just the tagged ones,
	// or all of them.
	vts := []version.Type{version.TypeRelease, version.TypePrerelease}
	if params.PseudoVersions {
		vts = append(vts, version.TypePseudo)
	}
	infos, next, err := ds.GetPathVersions(r.Context(), path, start, limit+1, vts...)
	if err != nil {
		return err
	}
	nextToken := ""
	if len(infos) > limit {
		if len(infos) != limit+1 {
			return InternalServerError("len(infos)=%d, expected %d", len(infos), limit+1)
		}
		infos = infos[:limit]
		nextToken, err = encodeStringPageToken(next)
		if err != nil {
			return err
		}
	}

	var mvs []ModuleVersion
	for _, in := range infos {
		mvs = append(mvs, ModuleVersion{
			ModulePath:        in.ModulePath,
			Version:           in.Version,
			CommitTime:        in.CommitTime,
			IsRedistributable: in.IsRedistributable,
			HasGoMod:          in.HasGoMod,
			LatestVersion:     in.LatestVersion,
			Deprecated:        in.Deprecated,
			DeprecationReason: in.DeprecationComment,
			Retracted:         in.Retracted,
			RetractionReason:  in.RetractionRationale,
		})
	}
	mvs, err = filterStruct(mvs, params.Filter)
	if err != nil {
		return err
	}

	// In general, we don't know the total number of versions. The only time we can be
	// sure is if this is the first page, and there is no next page.
	// In that case, we count the number of filtered results.
	total := -1
	if start == "" && nextToken == "" {
		total = len(mvs)
	}

	// api:response PaginatedResponse[ModuleVersion]
	resp := PaginatedResponse[ModuleVersion]{
		Items:         mvs,
		Total:         total,
		NextPageToken: nextToken,
	}

	// The response is never immutable, because a new version can arrive at any time.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServeModulePackages handles requests for the v1beta module packages endpoint.
// api:route /v1beta/packages/{path}
// api:desc Information about packages of the module at {path}.
// api:desc Filtering is applied to the list of packages in the response.
// api:desc Only packages that match the filter query parameter are returned.
// api:example /v1beta/packages/golang.org/x/time/rate
func ServeModulePackages(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModulePackages")

	// api:pathparam path Module path.
	modulePath := trimPath(r, "/v1beta/packages/")
	if modulePath == "" {
		return BadRequest("missing module path",
			"the module path must be provided after '/packages/'")
	}

	// api:params PackagesParams
	var params PackagesParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	// Resolve latest version, and check if specific version exists.
	um, err := ds.GetUnitMeta(r.Context(), modulePath, internal.UnknownModulePath, requestedVersion)
	if err != nil {
		return err
	}

	if err := checkModulePath(modulePath, um.ModulePath); err != nil {
		return err
	}

	metas, err := ds.GetModulePackages(r.Context(), um.ModulePath, um.Version)
	if err != nil {
		return err
	}

	var pinfos []PackageInfo
	for _, m := range metas {
		pinfos = append(pinfos, PackageInfo{
			Path:              m.Path,
			Name:              m.Name,
			Synopsis:          m.Synopsis,
			IsRedistributable: m.IsRedistributable,
		})
	}
	pinfos, err = filterStruct(pinfos, params.Filter)
	if err != nil {
		return err
	}

	// api:response PackagesResponse
	resp := PackagesResponse{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		IsStandardLibrary: stdlib.Contains(modulePath),
	}

	resp.Packages, err = paginate(pinfos, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(requestedVersion))
}

// ServeSearch handles requests for the v1 search endpoint.
// api:route /v1beta/search
// api:desc Search results. Only results that match the filter query parameter are returned.
// api:desc Results are sorted by how well the match the query, with the best match first.
// api:example /v1beta/search?q=xyzzy
func ServeSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeSearch")

	// api:params SearchParams
	var params SearchParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	if params.Query == "" {
		return BadRequest("missing query", "provide the query by using the query parameter 'q'")
	}

	dbLimit := maxSearchResults
	// We can only optimize the DB limit when no filter is present.
	// Filtering is done in memory after fetching results, so we need a large
	// candidate set to avoid returning empty pages when matches exist further down.
	if params.Filter == "" {
		limit, offset, err := params.ListParams.pageParams(defaultSearchLimit)
		if err != nil {
			return fmt.Errorf("%w: %s", derrors.InvalidArgument, err.Error())
		}
		dbLimit = min(offset+limit+1, maxSearchResults)
	}

	dbresults, err := ds.Search(r.Context(), params.Query, internal.SearchOptions{
		MaxResults:     dbLimit,
		MaxResultCount: maxSearchResults,
		SearchSymbols:  params.Symbol != "",
		SymbolFilter:   params.Symbol,
		// Don't group search results: packages in the same module and
		// in modules with different major versions will all appear in
		// the same flat list, sorted by score.
		GroupResults: false,
	})
	if err != nil {
		return err
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

	results, err = filterStruct(results, params.Filter)
	if err != nil {
		return err
	}

	// api:response PaginatedResponse[SearchResult]
	resp, err := paginate(results, params.ListParams, defaultLimit)
	if err != nil {
		return fmt.Errorf("%w: %s", derrors.InvalidArgument, err.Error())
	}
	if params.Filter == "" && len(dbresults) > 0 {
		resp.Total = int(dbresults[0].NumResults)
	}

	// Search results are never immutable, because new modules are always being added.
	// NOTE: the default cache freshness is set to 1 hour (see serveJSON). This seems
	// like a reasonable time to cache a search, but be aware of complaints
	// about stale search results.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServePackageSymbols handles requests for the v1beta package symbols endpoint.
// api:route /v1beta/symbols/{path}
// api:desc List of symbols for the package at {path}.
// api:desc Filtering is applied to the list of symbols in the response.
// api:desc Only symbols that match the filter query parameter are returned.
// api:example /v1beta/symbols/golang.org/x/time/rate
func ServePackageSymbols(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageSymbols")

	// api:pathparam path Package path.
	pkgPath := trimPath(r, "/v1beta/symbols/")
	if pkgPath == "" {
		return BadRequest("missing package path",
			"the package path must be provided after '/symbols/'")
	}

	// api:params SymbolsParams
	var params SymbolsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	um, err := resolveModulePath(r, ds, pkgPath, params.Module, params.Version)
	if err != nil {
		return err
	}

	bc := internal.BuildContext{GOOS: params.GOOS, GOARCH: params.GOARCH}
	dbSyms, err := ds.GetSymbols(r.Context(), pkgPath, um.ModulePath, um.Version, bc)
	if err != nil {
		return fmt.Errorf("symbols for package %s: %w", pkgPath, err)
	}

	var syms []*internal.Symbol
	for _, s := range dbSyms {
		syms = append(syms, s)
		for _, child := range s.Children {
			syms = append(syms, &internal.Symbol{
				SymbolMeta: *child,
				GOOS:       s.GOOS,
				GOARCH:     s.GOARCH,
			})
		}
		s.Children = nil
	}

	syms = slices.Clone(syms)

	// TODO(jba): combine this loop with the one above, if possible.
	var items []Symbol
	for _, s := range syms {
		items = append(items, Symbol{
			Name:     s.Name,
			Kind:     string(s.Kind),
			Synopsis: s.Synopsis,
			Parent:   s.ParentName,
		})
	}

	items, err = filterStruct(items, params.Filter)
	if err != nil {
		return err
	}

	paged, err := paginate(items, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}
	// api:response PackageSymbols
	resp := PackageSymbols{
		ModulePath: um.ModulePath,
		Version:    um.Version,
		Symbols:    paged,
	}

	return serveJSON(w, http.StatusOK, resp, versionCacheDur(params.Version))
}

// ServePackageImportedBy handles requests for the v1beta package imported-by endpoint.
// api:route /v1beta/imported-by/{path}
// api:desc Paths of packages importing the package at {path},
// api:desc not including packages in the same module.
// api:desc Filtering is applied to the list of paths in the response.
// api:desc Only paths that match the filter query parameter are returned.
// api:desc Within a filter, the variable `path` is set to the import path.
// api:example /v1beta/imported-by/golang.org/x/time/rate?limit=10&filter=%5E.%2A%5C.io%2F
func ServePackageImportedBy(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageImportedBy")

	// api:pathparam path Package path.
	pkgPath := trimPath(r, "/v1beta/imported-by/")
	if pkgPath == "" {
		return BadRequest("missing package path",
			"the package path must be provided after '/imported-by/'")
	}

	// api:params ImportedByParams
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

	// TODO(jba): share limit and start code between this
	// and versions.
	limit := params.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	start := ""
	if params.Token != "" {
		var err error
		start, err = decodeStringPageToken(params.Token)
		if err != nil {
			return BadRequest(fmt.Sprintf("invalid next-page token: %v", err), "try again from the beginning, with no token")
		}
	}

	// Fetch an extra item so we can tell if we're done.
	importedBy, err := ds.GetImportedBy(r.Context(), pkgPath, modulePath, start, limit+1)
	if err != nil {
		return err
	}

	nextToken := ""
	if len(importedBy) > limit {
		if len(importedBy) != limit+1 {
			return InternalServerError("len(importedBy)=%d, expected %d", len(importedBy), limit+1)
		}

		nextToken, err = encodeStringPageToken(importedBy[limit])
		if err != nil {
			return err
		}
		importedBy = importedBy[:limit]
	}

	count, err := ds.GetImportedByCount(r.Context(), pkgPath, modulePath)
	if err != nil {
		return err
	}

	filtered, err := filterString(importedBy, params.Filter, "path")
	if err != nil {
		return err
	}
	// len(filtered) may be 0. That's fine: we document that zero-length
	// pages are OK.
	// The alternative is to fetch rows indefinitely, which means unbounded
	// work.

	// api:response PackageImportedBy
	resp := PackageImportedBy{
		ModulePath: modulePath,
		Version:    requestedVersion,
		ImportedBy: PaginatedResponse[string]{
			Items:         filtered,
			Total:         count,
			NextPageToken: nextToken,
		},
	}

	// The imported-by list is not immutable, because new modules are always being added.
	return serveJSON(w, http.StatusOK, resp, shortCacheDur)
}

// ServeVulnerabilities handles requests for the v1beta vulnerabilities endpoint.
// api:route /v1beta/vulns/{path}
// api:desc Vulnerabilities of the module or package at {path}.
// api:desc Data comes from the Go vulnerability database (https://vuln.go.dev).
// api:desc Only results that match the filter query parameter are returned.
// api:example /v1beta/vulns/golang.org/x/image
func ServeVulnerabilities(vc *vuln.Client) func(w http.ResponseWriter, r *http.Request, _ internal.DataSource) error {
	return func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
		defer derrors.Wrap(&err, "ServeVulnerabilities")

		// api:pathparam path Module or package path.
		path := trimPath(r, "/v1beta/vulns/")
		if path == "" {
			return BadRequest("missing path",
				"the package or module path must be provided after '/vulns/'")
		}

		// api:params VulnParams
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

		// Verify package or module existence and resolve containing module.
		um, err := resolveModulePath(r, ds, path, params.Module, requestedVersion)
		if err != nil {
			return err
		}

		var pkgPath string
		if path != um.ModulePath {
			pkgPath = path
		}

		// Use VulnsForPackage from internal/vuln to get vulnerabilities.
		// If pkgPath is non-empty, it filters vulnerabilities to only that package.
		vulns := vuln.VulnsForPackage(r.Context(), um.ModulePath, um.Version, pkgPath, vc)

		vulns, err = filterStruct(vulns, params.Filter)
		if err != nil {
			return err
		}

		var items []Vulnerability
		for _, v := range vulns {
			items = append(items, Vulnerability{
				ID:      v.ID,
				Details: v.Details,
			})
		}

		// api:response PaginatedResponse[Vulnerability]
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
			Fixes:      []string{"retry the call with a 'module' query parameter specifying the desired module"},
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
	limit, offset, err := lp.pageParams(defaultLimit)
	if err != nil {
		return PaginatedResponse[T]{}, fmt.Errorf("%w: %s", derrors.InvalidArgument, err)
	}

	offset = min(offset, len(all))
	end := min(offset+limit, len(all))

	var nextToken string
	if end < len(all) {
		var err error
		nextToken, err = encodePageToken(end)
		if err != nil {
			return PaginatedResponse[T]{}, fmt.Errorf("encoding token: %w", err)
		}
	}

	return PaginatedResponse[T]{
		Items:         all[offset:end],
		Total:         len(all),
		NextPageToken: nextToken,
	}, nil
}

// unitToPackage processes unit documentation into a Package struct.
func unitToPackage(unit *internal.Unit, params PackageParams) (*Package, error) {
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
		ModulePath:        unit.ModulePath,
		Version:           unit.Version,
		IsStandardLibrary: stdlib.Contains(unit.ModulePath),
		IsLatest:          unit.Version == unit.LatestVersion,
		GOOS:              goos,
		GOARCH:            goarch,
		Docs:              docs,
		Imports:           unit.Imports,
		Licenses:          licenses,
		PackageInfo: PackageInfo{
			Path:              unit.Path,
			Name:              unit.Name,
			IsRedistributable: unit.IsRedistributable,
			Synopsis:          synopsis,
		},
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
		// ServePackage needs to be updated when the doc set changes.
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

// filter returns a new slice containing all elements in list which match
// the expression denoted by filter.
// It sets each element to varName before evaluating the filter.
func filterString(list []string, filter, varName string) ([]string, error) {
	if varName == "" {
		return nil, errors.New("string filter must have varName")
	}
	if filter == "" {
		return list, nil
	}
	return filterInternal(list, filter, nil, varName)
}

// filter returns a new slice containing all elements in list which match
// the expression denoted by filter.
func filterStruct[T any](list []T, filter string) ([]T, error) {
	if filter == "" {
		return list, nil
	}
	t := reflect.TypeFor[T]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("bad type %s for filter: need struct or pointer to struct", t)
	}
	return filterInternal(list, filter, jsonFields(t), "")
}

func filterInternal[T any](list []T, filter string, jfields fieldMap, varName string) ([]T, error) {
	expr, err := parser.ParseExpr(filter)
	if err != nil {
		return nil, BadRequest(fmt.Sprintf(`parsing filter "%s": %v`,
			filter, err),
			"the 'filter' query parameter must be a valid Go expression; see the documentation at /v1beta/api",
		)
	}
	var out []T
	for _, e := range list {
		env := maps.Clone(defaultEnv)
		if jfields == nil {
			env[varName] = e
		} else {
			tv := reflect.ValueOf(e)
			if !tv.IsValid() {
				continue
			}
			for tv.Kind() == reflect.Pointer {
				tv = tv.Elem()
			}
			for name, field := range jfields {
				env[name] = tv.FieldByIndex(field.Index).Interface()
			}
		}
		res, err := evaluate(expr, env)
		if err != nil {
			return nil, BadRequest(fmt.Sprintf(`evaluating filter "%s": %v`, filter, err),
				"the filter must be a Go expression; see the documentation at /v1beta/api")
		}
		b, ok := res.(bool)
		if !ok {
			return nil, BadRequest(fmt.Sprintf(`filter "%s" did not evaluate to bool`, filter),
				"the filter must be a boolean Go expression; see the documentation at /v1beta/api")
		}
		if b {
			out = append(out, e)
		}
	}
	return out, nil
}

// checkModulePath verifies that the requested module path exactly matches the resolved
// module path. If it is a package path instead, it returns a BadRequest error with
// containing module suggestions.
func checkModulePath(requested, resolved string) error {
	if requested != resolved {
		return &Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("%s is a package, not a module", requested),
			Fixes:   []string{fmt.Sprintf("retry the call with the containing module: %q", resolved)},
		}
	}
	return nil
}
