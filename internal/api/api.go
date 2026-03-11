// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/pkgsite/internal/vuln"
)

const (
	// maxSearchResults is the maximum number of search results to return for a search query.
	maxSearchResults = 1000
	// searchResultsPerPage is the number of search results to return per page for paginated search results.
	searchResultsPerPage = 100
)

// ServePackage handles requests for the v1 package metadata endpoint.
func ServePackage(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackage")

	// The path is expected to be /v1/package/{path}
	pkgPath := strings.TrimPrefix(r.URL.Path, "/v1/package/")
	pkgPath = strings.Trim(pkgPath, "/")
	if pkgPath == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing package path", nil)
	}

	var params PackageParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	um, candidates, err := resolveModulePath(r, ds, pkgPath, params.Module, params.Version)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}
	if len(candidates) > 0 {
		return serveErrorJSON(w, http.StatusBadRequest, "ambiguous package path", candidates)
	}

	// Use GetUnit to get the requested data.
	fs := internal.WithMain
	if params.Licenses {
		fs |= internal.WithLicenses
	}
	if params.Imports {
		fs |= internal.WithImports
	}
	if params.Doc != "" {
		fs |= internal.WithDocsSource
	}

	bc := internal.BuildContext{GOOS: params.GOOS, GOARCH: params.GOARCH}
	unit, err := ds.GetUnit(r.Context(), um, fs, bc)
	if err != nil {
		return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
	}

	// Process documentation, including synopsis.
	// Although unit.Documentation is a slice, it will
	// have at most one item, the documentation matching
	// the build context bc.
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
			// d.Source is an encoded AST. Decode it, then use
			// go/doc (not pkgsite's renderer) to generate the
			// result.
			gpkg, err := godoc.DecodePackage(d.Source)
			if err != nil {
				return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
			}
			innerPath := internal.Suffix(unit.Path, unit.ModulePath)
			modInfo := &godoc.ModuleInfo{ModulePath: unit.ModulePath, ResolvedVersion: unit.Version}
			dpkg, err := gpkg.DocPackage(innerPath, modInfo)
			if err != nil {
				return err
			}
			var r renderer
			var sb strings.Builder
			switch params.Doc {
			case "text":
				r = &textRenderer{fset: gpkg.Fset, w: &sb}
			case "md", "markdown":
				r = &markdownRenderer{fset: gpkg.Fset, w: &sb}
			case "html":
				return errors.New("unimplemented")
			default:
				return serveErrorJSON(w, http.StatusBadRequest, "bad doc format: need one of 'text', 'md', 'markdown' or 'html'", nil)
			}
			if err := renderDoc(dpkg, r); err != nil {
				return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
			}
			docs = sb.String()
		}
	}

	imports := unit.Imports
	var licenses []License
	for _, l := range unit.LicenseContents {
		licenses = append(licenses, License{
			Types:    l.Metadata.Types,
			FilePath: l.Metadata.FilePath,
			Contents: string(l.Contents),
		})
	}

	resp := Package{
		Path:              unit.Path,
		ModulePath:        unit.ModulePath,
		ModuleVersion:     unit.Version,
		Synopsis:          synopsis,
		IsStandardLibrary: stdlib.Contains(unit.ModulePath),
		IsLatest:          unit.Version == unit.LatestVersion,
		GOOS:              goos,
		GOARCH:            goarch,
		Docs:              docs,
		Imports:           imports,
		Licenses:          licenses,
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModule handles requests for the v1 module metadata endpoint.
func ServeModule(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModule")

	modulePath := strings.TrimPrefix(r.URL.Path, "/v1/module/")
	modulePath = strings.Trim(modulePath, "/")
	if modulePath == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing module path", nil)
	}

	var params ModuleParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	// For modules, we can use GetUnitMeta on the module path.
	um, err := ds.GetUnitMeta(r.Context(), modulePath, modulePath, requestedVersion)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}

	resp := Module{
		Path:              um.ModulePath,
		Version:           um.Version,
		IsStandardLibrary: stdlib.Contains(um.ModulePath),
		IsRedistributable: um.IsRedistributable,
	}
	// RepoURL needs to be extracted from source info if available
	if um.SourceInfo != nil {
		resp.RepoURL = um.SourceInfo.RepoURL()
	}

	if params.Readme {
		readme, err := ds.GetModuleReadme(r.Context(), um.ModulePath, um.Version)
		if err == nil && readme != nil {
			resp.Readme = &Readme{
				Filepath: readme.Filepath,
				Contents: readme.Contents,
			}
		}
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModuleVersions handles requests for the v1 module versions endpoint.
func ServeModuleVersions(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModuleVersions")

	path := strings.TrimPrefix(r.URL.Path, "/v1/versions/")
	if path == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing path", nil)
	}

	var params VersionsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	infos, err := ds.GetVersionsForPath(r.Context(), path)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}

	resp, err := paginate(infos, params.ListParams, 100)
	if err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModulePackages handles requests for the v1 module packages endpoint.
func ServeModulePackages(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModulePackages")

	modulePath := strings.TrimPrefix(r.URL.Path, "/v1/packages/")
	if modulePath == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing module path", nil)
	}

	var params PackagesParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	metas, err := ds.GetModulePackages(r.Context(), modulePath, requestedVersion)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}

	// TODO: Handle params.Token and params.Filter.
	// For now, we just use params.Limit to limit the number of packages returned.
	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > len(metas) {
		limit = len(metas)
	}

	var items []Package
	for _, m := range metas[:limit] {
		items = append(items, Package{
			Path:              m.Path,
			ModulePath:        modulePath,
			ModuleVersion:     requestedVersion,
			Synopsis:          m.Synopsis,
			IsStandardLibrary: stdlib.Contains(modulePath),
		})
	}

	resp := PaginatedResponse[Package]{
		Items: items,
		Total: len(metas),
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeSearch handles requests for the v1 search endpoint.
func ServeSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeSearch")

	var params SearchParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	if params.Query == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing query", nil)
	}

	dbresults, err := ds.Search(r.Context(), params.Query, internal.SearchOptions{
		MaxResults:    maxSearchResults,
		SearchSymbols: params.Symbol != "",
		SymbolFilter:  params.Symbol,
	})
	if err != nil {
		return err
	}

	var results []SearchResult
	for _, r := range dbresults {
		if params.Filter != "" {
			if !strings.Contains(r.Synopsis, params.Filter) && !strings.Contains(r.PackagePath, params.Filter) {
				continue
			}
		}
		results = append(results, SearchResult{
			PackagePath: r.PackagePath,
			ModulePath:  r.ModulePath,
			Version:     r.Version,
			Synopsis:    r.Synopsis,
		})
	}

	resp, err := paginate(results, params.ListParams, searchResultsPerPage)
	if err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServePackageSymbols handles requests for the v1 package symbols endpoint.
func ServePackageSymbols(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageSymbols")

	pkgPath := strings.TrimPrefix(r.URL.Path, "/v1/symbols/")
	pkgPath = strings.Trim(pkgPath, "/")
	if pkgPath == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing package path", nil)
	}

	var params SymbolsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	um, candidates, err := resolveModulePath(r, ds, pkgPath, params.Module, params.Version)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}
	if len(candidates) > 0 {
		return serveErrorJSON(w, http.StatusBadRequest, "ambiguous package path", candidates)
	}

	bc := internal.BuildContext{GOOS: params.GOOS, GOARCH: params.GOARCH}
	syms, err := ds.GetSymbols(r.Context(), pkgPath, um.ModulePath, um.Version, bc)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > len(syms) {
		limit = len(syms)
	}

	var items []Symbol
	for _, s := range syms[:limit] {
		items = append(items, Symbol{
			ModulePath: um.ModulePath,
			Version:    um.Version,
			Name:       s.Name,
			Kind:       string(s.Kind),
			Synopsis:   s.Synopsis,
			Parent:     s.ParentName,
		})
	}

	resp := PaginatedResponse[Symbol]{
		Items: items,
		Total: len(syms),
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServePackageImportedBy handles requests for the v1 package imported-by endpoint.
func ServePackageImportedBy(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageImportedBy")

	pkgPath := strings.TrimPrefix(r.URL.Path, "/v1/imported-by/")
	if pkgPath == "" {
		return serveErrorJSON(w, http.StatusBadRequest, "missing package path", nil)
	}

	var params ImportedByParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	um, candidates, err := resolveModulePath(r, ds, pkgPath, params.Module, requestedVersion)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}
	if len(candidates) > 0 {
		return serveErrorJSON(w, http.StatusBadRequest, "ambiguous package path", candidates)
	}
	modulePath := um.ModulePath

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	importedBy, err := ds.GetImportedBy(r.Context(), pkgPath, modulePath, limit)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
		}
		return err
	}

	count, err := ds.GetImportedByCount(r.Context(), pkgPath, modulePath)
	if err != nil {
		return err
	}

	resp := PackageImportedBy{
		ModulePath: modulePath,
		Version:    requestedVersion,
		ImportedBy: PaginatedResponse[string]{
			Items: importedBy,
			Total: count,
		},
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeVulnerabilities handles requests for the v1 module vulnerabilities endpoint.
func ServeVulnerabilities(vc *vuln.Client) func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	return func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
		defer derrors.Wrap(&err, "ServeVulnerabilities")

		modulePath := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
		if modulePath == "" {
			return serveErrorJSON(w, http.StatusBadRequest, "missing module path", nil)
		}

		var params VulnParams
		if err := ParseParams(r.URL.Query(), &params); err != nil {
			return serveErrorJSON(w, http.StatusBadRequest, err.Error(), nil)
		}

		if vc == nil {
			return serveErrorJSON(w, http.StatusNotImplemented, "vulnerability data not available", nil)
		}

		requestedVersion := params.Version
		if requestedVersion == "" {
			requestedVersion = version.Latest
		}

		// Use VulnsForPackage from internal/vuln to get vulnerabilities for the module.
		// Passing an empty packagePath gets all vulns for the module.
		vulns := vuln.VulnsForPackage(r.Context(), modulePath, requestedVersion, "", vc)

		limit := params.Limit
		if limit <= 0 {
			limit = 100
		}
		if limit > len(vulns) {
			limit = len(vulns)
		}

		var items []Vulnerability
		for _, v := range vulns[:limit] {
			items = append(items, Vulnerability{
				ID:      v.ID,
				Details: v.Details,
			})
		}

		resp := PaginatedResponse[Vulnerability]{
			Items: items,
			Total: len(vulns),
		}

		return serveJSON(w, http.StatusOK, resp)
	}
}

// resolveModulePath determines the correct module path for a given package path and version.
// If the module path is not provided, it searches through potential candidate module paths
// derived from the package path. If multiple valid modules contain the package, it returns
// a list of candidates to help the user disambiguate the request.
func resolveModulePath(r *http.Request, ds internal.DataSource, pkgPath, modulePath, requestedVersion string) (*internal.UnitMeta, []Candidate, error) {
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}
	if modulePath == "" {
		// Handle potential ambiguity if module is not specified.
		candidates := internal.CandidateModulePaths(pkgPath)
		var validCandidates []Candidate
		var foundUM *internal.UnitMeta
		for _, mp := range candidates {
			// Check if this module actually exists and contains the package at the requested version.
			if m, err := ds.GetUnitMeta(r.Context(), pkgPath, mp, requestedVersion); err == nil {
				foundUM = m
				validCandidates = append(validCandidates, Candidate{
					ModulePath:  mp,
					PackagePath: pkgPath,
				})
			} else if !errors.Is(err, derrors.NotFound) {
				return nil, nil, err
			}
		}

		if len(validCandidates) > 1 {
			return nil, validCandidates, nil
		}
		if len(validCandidates) == 0 {
			return nil, nil, derrors.NotFound
		}
		return foundUM, nil, nil
	}

	um, err := ds.GetUnitMeta(r.Context(), pkgPath, modulePath, requestedVersion)
	if err != nil {
		return nil, nil, err
	}
	return um, nil, nil
}

func serveJSON(w http.ResponseWriter, status int, data any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(buf.Bytes())
	return err
}

func serveErrorJSON(w http.ResponseWriter, status int, message string, candidates []Candidate) error {
	return serveJSON(w, status, Error{
		Code:       status,
		Message:    message,
		Candidates: candidates,
	})
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
			return PaginatedResponse[T]{}, errors.New("invalid token")
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
