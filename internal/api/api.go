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
	// defaultLimit is the default number of results to return per page for paginated results.
	defaultLimit = 100
)

// ServePackage handles requests for the v1 package metadata endpoint.
func ServePackage(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackage")

	pkgPath := trimPath(r, "/v1/package/")
	if pkgPath == "" {
		return fmt.Errorf("%w: missing package path", derrors.InvalidArgument)
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
		return fmt.Errorf("%w: %s", derrors.Unknown, err.Error())
	}

	resp, err := unitToPackage(unit, params)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModule handles requests for the v1 module metadata endpoint.
func ServeModule(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModule")

	modulePath := trimPath(r, "/v1/module/")
	if modulePath == "" {
		return fmt.Errorf("%w: missing module path", derrors.InvalidArgument)
	}

	var params ModuleParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	requestedVersion := params.Version
	if requestedVersion == "" {
		requestedVersion = version.Latest
	}

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
		return serveJSON(w, http.StatusOK, resp)
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
		return serveJSON(w, http.StatusOK, resp)
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

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModuleVersions handles requests for the v1 module versions endpoint.
func ServeModuleVersions(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModuleVersions")

	path := trimPath(r, "/v1/versions/")
	if path == "" {
		return fmt.Errorf("%w: missing path", derrors.InvalidArgument)
	}

	var params VersionsParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return err
	}

	infos, err := ds.GetVersionsForPath(r.Context(), path)
	if err != nil {
		return err
	}

	if params.Filter != "" {
		var filtered []*internal.ModuleInfo
		for _, info := range infos {
			if strings.Contains(info.Version, params.Filter) {
				filtered = append(filtered, info)
			}
		}
		infos = filtered
	}

	resp, err := paginate(infos, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeModulePackages handles requests for the v1 module packages endpoint.
func ServeModulePackages(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeModulePackages")

	modulePath := trimPath(r, "/v1/packages/")
	if modulePath == "" {
		return fmt.Errorf("%w: missing module path", derrors.InvalidArgument)
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

	var items []Package
	for _, m := range metas {
		if params.Filter != "" && !strings.Contains(m.Path, params.Filter) && !strings.Contains(m.Synopsis, params.Filter) {
			continue
		}
		items = append(items, Package{
			Path:              m.Path,
			ModulePath:        modulePath,
			ModuleVersion:     requestedVersion,
			Synopsis:          m.Synopsis,
			IsStandardLibrary: stdlib.Contains(modulePath),
		})
	}

	resp, err := paginate(items, params.ListParams, defaultLimit)
	if err != nil {
		return err
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServeSearch handles requests for the v1 search endpoint.
func ServeSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServeSearch")

	var params SearchParams
	if err := ParseParams(r.URL.Query(), &params); err != nil {
		return fmt.Errorf("%w: %s", derrors.InvalidArgument, err.Error())
	}

	if params.Query == "" {
		return fmt.Errorf("%w: missing query", derrors.InvalidArgument)
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

	resp, err := paginate(results, params.ListParams, defaultLimit)
	if err != nil {
		return fmt.Errorf("%w: %s", derrors.InvalidArgument, err.Error())
	}

	return serveJSON(w, http.StatusOK, resp)
}

// ServePackageSymbols handles requests for the v1 package symbols endpoint.
func ServePackageSymbols(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageSymbols")

	pkgPath := trimPath(r, "/v1/symbols/")
	if pkgPath == "" {
		return fmt.Errorf("%w: missing package path", derrors.InvalidArgument)
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

	var items []Symbol
	for _, s := range syms {
		if params.Filter != "" && !strings.Contains(s.Name, params.Filter) && !strings.Contains(s.Synopsis, params.Filter) {
			continue
		}
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

	return serveJSON(w, http.StatusOK, resp)
}

// ServePackageImportedBy handles requests for the v1 package imported-by endpoint.
func ServePackageImportedBy(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "ServePackageImportedBy")

	pkgPath := trimPath(r, "/v1/imported-by/")
	if pkgPath == "" {
		return fmt.Errorf("%w: missing package path", derrors.InvalidArgument)
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
		var filtered []string
		for _, p := range importedBy {
			if strings.Contains(p, params.Filter) {
				filtered = append(filtered, p)
			}
		}
		importedBy = filtered
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

	return serveJSON(w, http.StatusOK, resp)
}

// ServeVulnerabilities handles requests for the v1 module vulnerabilities endpoint.
func ServeVulnerabilities(vc *vuln.Client) func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	return func(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
		defer derrors.Wrap(&err, "ServeVulnerabilities")

		modulePath := trimPath(r, "/v1/vulns/")
		if modulePath == "" {
			return fmt.Errorf("%w: missing module path", derrors.InvalidArgument)
		}

		var params VulnParams
		if err := ParseParams(r.URL.Query(), &params); err != nil {
			return err
		}

		if vc == nil {
			return fmt.Errorf("%w: vulnerability data not available", derrors.Unsupported)
		}

		requestedVersion := params.Version
		if requestedVersion == "" {
			requestedVersion = version.Latest
		}

		// Use VulnsForPackage from internal/vuln to get vulnerabilities for the module.
		// Passing an empty packagePath gets all vulns for the module.
		vulns := vuln.VulnsForPackage(r.Context(), modulePath, requestedVersion, "", vc)

		var items []Vulnerability
		for _, v := range vulns {
			if params.Filter != "" && !strings.Contains(v.ID, params.Filter) && !strings.Contains(v.Details, params.Filter) {
				continue
			}
			items = append(items, Vulnerability{
				ID:      v.ID,
				Details: v.Details,
			})
		}

		resp, err := paginate(items, params.ListParams, defaultLimit)
		if err != nil {
			return err
		}

		return serveJSON(w, http.StatusOK, resp)
	}
}

func trimPath(r *http.Request, prefix string) string {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	return strings.Trim(path, "/")
}

// resolveModulePath determines the correct module path for a given package path and version.
// If the module path is not provided, it searches through potential candidate module paths
// derived from the package path. If multiple valid modules contain the package, it returns
// a list of candidates to help the user disambiguate the request.
func resolveModulePath(r *http.Request, ds internal.DataSource, pkgPath, modulePath, requestedVersion string) (*internal.UnitMeta, error) {
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
				return nil, err
			}
		}

		if len(validCandidates) > 1 {
			return nil, &Error{
				Code:       http.StatusBadRequest,
				Message:    "ambiguous package path",
				Candidates: validCandidates,
			}
		}
		if len(validCandidates) == 0 {
			return nil, derrors.NotFound
		}
		return foundUM, nil
	}

	um, err := ds.GetUnitMeta(r.Context(), pkgPath, modulePath, requestedVersion)
	if err != nil {
		return nil, err
	}
	return um, nil
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

func ServeError(w http.ResponseWriter, err error) error {
	var aerr *Error
	if !errors.As(err, &aerr) {
		status := derrors.ToStatus(err)
		aerr = &Error{
			Code:    status,
			Message: err.Error(),
		}
	}
	return serveJSON(w, aerr.Code, aerr)
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
		return nil, fmt.Errorf("%w: examples require doc format to be specified", derrors.InvalidArgument)
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
		return "", fmt.Errorf("%w: %s", derrors.Unknown, err.Error())
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
		return "", fmt.Errorf("%w: bad doc format: need one of 'text', 'md', 'markdown' or 'html'", derrors.InvalidArgument)
	}
	if err := renderDoc(dpkg, r, examples); err != nil {
		return "", fmt.Errorf("%w: %s", derrors.Unknown, err.Error())
	}
	return sb.String(), nil
}
