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

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

	var um *internal.UnitMeta
	modulePath := params.Module
	if modulePath == "" {
		// Handle potential ambiguity if module is not specified.
		candidates := internal.CandidateModulePaths(pkgPath)
		var validCandidates []Candidate
		for _, mp := range candidates {
			// Check if this module actually exists and contains the package at the requested version.
			if m, err := ds.GetUnitMeta(r.Context(), pkgPath, mp, requestedVersion); err == nil {
				um = m
				validCandidates = append(validCandidates, Candidate{
					ModulePath:  mp,
					PackagePath: pkgPath,
				})
			} else if !errors.Is(err, derrors.NotFound) {
				return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
			}
		}

		if len(validCandidates) > 1 {
			return serveErrorJSON(w, http.StatusBadRequest, "ambiguous package path", validCandidates)
		}
		if len(validCandidates) == 0 {
			return serveErrorJSON(w, http.StatusNotFound, "package not found", nil)
		}
		modulePath = validCandidates[0].ModulePath
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
	var unit *internal.Unit
	if um != nil {
		var err error
		unit, err = ds.GetUnit(r.Context(), um, fs, bc)
		if err != nil {
			return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
		}
	} else if modulePath != "" && modulePath != internal.UnknownModulePath && !needsResolution(requestedVersion) {
		// This block is reachable if the user explicitly provided a module path and a
		// concrete version in the query parameters, skipping the candidate search.
		um = &internal.UnitMeta{
			Path: pkgPath,
			ModuleInfo: internal.ModuleInfo{
				ModulePath: modulePath,
				Version:    requestedVersion,
			},
		}
		var err error
		unit, err = ds.GetUnit(r.Context(), um, fs, bc)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
		}
	}

	if unit == nil {
		// Fallback: Resolve the version or find the module using GetUnitMeta.
		var err error
		um, err = ds.GetUnitMeta(r.Context(), pkgPath, modulePath, requestedVersion)
		if err != nil {
			if errors.Is(err, derrors.NotFound) {
				return serveErrorJSON(w, http.StatusNotFound, err.Error(), nil)
			}
			return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
		}
		unit, err = ds.GetUnit(r.Context(), um, fs, bc)
		if err != nil {
			return serveErrorJSON(w, http.StatusInternalServerError, err.Error(), nil)
		}
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
			var tr renderer
			var sb strings.Builder
			switch params.Doc {
			case "text":
				tr = &textRenderer{fset: gpkg.Fset, w: &sb}
			case "md", "markdown":
				return errors.New("unimplemented")
			case "html":
				return errors.New("unimplemented")
			default:
				return serveErrorJSON(w, http.StatusBadRequest, "bad doc format: need one of 'text', 'md', 'markdown' or 'html'", nil)
			}
			if err := renderDoc(dpkg, tr); err != nil {
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

// needsResolution reports whether the version string is a sentinel like "latest" or "master".
func needsResolution(v string) bool {
	return v == version.Latest || v == version.Master || v == version.Main
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
