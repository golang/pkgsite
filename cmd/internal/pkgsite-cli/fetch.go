// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A client fetches data from the pkg.go.dev v1 API.
type client struct {
	server     string
	httpClient *http.Client
}

func newClient(server string) *client {
	return &client{
		server:     server,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// apiError is the error format returned by the v1 API.
type apiError struct {
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Candidates []candidate `json:"candidates,omitempty"`
}

type candidate struct {
	ModulePath  string `json:"modulePath"`
	PackagePath string `json:"packagePath"`
}

func (e *apiError) Error() string {
	if len(e.Candidates) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s; specify --module flag:\n", e.Message)
		for _, c := range e.Candidates {
			fmt.Fprintf(&b, "  --module=%s\n", c.ModulePath)
		}
		return b.String()
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Code)
}

// get fetches url and decodes the JSON response into dst.
func (c *client) get(url string, dst any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pkgsite-cli/v1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit to 1MB
		if err != nil {
			return fmt.Errorf("reading error response: %w", err)
		}
		var aerr apiError
		if json.Unmarshal(body, &aerr) == nil && aerr.Message != "" {
			return &aerr
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// packageResponse is the JSON response for /v1/package/.
// Field names match internal/api.Package.
type packageResponse struct {
	Path              string            `json:"path"`
	ModulePath        string            `json:"modulePath"`
	ModuleVersion     string            `json:"moduleVersion"`
	Synopsis          string            `json:"synopsis"`
	IsStandardLibrary bool              `json:"isStandardLibrary"`
	IsLatest          bool              `json:"isLatest"`
	GOOS              string            `json:"goos"`
	GOARCH            string            `json:"goarch"`
	Docs              string            `json:"docs,omitempty"`
	Imports           []string          `json:"imports,omitempty"`
	Licenses          []licenseResponse `json:"licenses,omitempty"`
}

type licenseResponse struct {
	Types    []string `json:"types"`
	FilePath string   `json:"filePath"`
	Contents string   `json:"contents,omitempty"`
}

func (c *client) getPackage(path, version string, f *packageFlags) (*packageResponse, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if f.module != "" {
		q.Set("module", f.module)
	}
	if f.doc != "" {
		q.Set("doc", f.doc)
	}
	if f.examples {
		q.Set("examples", "true")
	}
	if f.imports {
		q.Set("imports", "true")
	}
	if f.licenses {
		q.Set("licenses", "true")
	}
	if f.goos != "" {
		q.Set("goos", f.goos)
	}
	if f.goarch != "" {
		q.Set("goarch", f.goarch)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "package", path)
	u.RawQuery = q.Encode()

	var resp packageResponse
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// paginatedResponse is a generic paginated response.
type paginatedResponse[T any] struct {
	Items         []T    `json:"items"`
	Total         int    `json:"total"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// symbolResponse is a single symbol from /v1/symbols/.
type symbolResponse struct {
	ModulePath string `json:"modulePath"`
	Version    string `json:"version"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Synopsis   string `json:"synopsis"`
	Parent     string `json:"parent,omitempty"`
}

func (c *client) getSymbols(path, version string, f *packageFlags) (*paginatedResponse[symbolResponse], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if f.module != "" {
		q.Set("module", f.module)
	}
	if f.goos != "" {
		q.Set("goos", f.goos)
	}
	if f.goarch != "" {
		q.Set("goarch", f.goarch)
	}
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "symbols", path)
	u.RawQuery = q.Encode()
	var resp paginatedResponse[symbolResponse]
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// importedByResponse is the response for /v1/imported-by/.
type importedByResponse struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	ImportedBy paginatedResponse[string] `json:"importedBy"`
}

func (c *client) getImportedBy(path, version string, f *packageFlags) (*importedByResponse, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if f.module != "" {
		q.Set("module", f.module)
	}
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "imported-by", path)
	u.RawQuery = q.Encode()
	var resp importedByResponse
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// moduleResponse is the JSON response for /v1/module/.
type moduleResponse struct {
	Path              string            `json:"path"`
	Version           string            `json:"version"`
	IsLatest          bool              `json:"isLatest"`
	IsRedistributable bool              `json:"isRedistributable"`
	IsStandardLibrary bool              `json:"isStandardLibrary"`
	HasGoMod          bool              `json:"hasGoMod"`
	RepoURL           string            `json:"repoUrl"`
	Readme            *readmeResponse   `json:"readme,omitempty"`
	Licenses          []licenseResponse `json:"licenses,omitempty"`
}

type readmeResponse struct {
	Filepath string `json:"filepath"`
	Contents string `json:"contents"`
}

func (c *client) getModule(path, version string, f *moduleFlags) (*moduleResponse, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if f.readme {
		q.Set("readme", "true")
	}
	if f.licenses {
		q.Set("licenses", "true")
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "module", path)
	u.RawQuery = q.Encode()
	var resp moduleResponse
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// versionResponse is a single version from /v1/versions/.
type versionResponse struct {
	Version string `json:"version"`
}

func (c *client) getVersions(path string, f *moduleFlags) (*paginatedResponse[versionResponse], error) {
	q := make(url.Values)
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "versions", path)
	u.RawQuery = q.Encode()
	var resp paginatedResponse[versionResponse]
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// vulnResponse is a single vulnerability from /v1/vulns/.
type vulnResponse struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}

func (c *client) getVulns(path, version string, f *moduleFlags) (*paginatedResponse[vulnResponse], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "vulns", path)
	u.RawQuery = q.Encode()
	var resp paginatedResponse[vulnResponse]
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// modulePackageResponse is a single package from /v1/packages/.
type modulePackageResponse struct {
	Path     string `json:"path"`
	Synopsis string `json:"synopsis"`
}

func (c *client) getPackages(modulePath, version string, f *moduleFlags) (*paginatedResponse[modulePackageResponse], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "packages", modulePath)
	u.RawQuery = q.Encode()
	var resp paginatedResponse[modulePackageResponse]
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// searchResultResponse is a single search result from /v1/search/.
type searchResultResponse struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

func (c *client) search(query string, f *searchFlags) (*paginatedResponse[searchResultResponse], error) {
	q := make(url.Values)
	q.Set("q", query)
	if f.symbol != "" {
		q.Set("symbol", f.symbol)
	}
	q.Set("limit", strconv.Itoa(f.effectiveLimit()))
	if f.token != "" {
		q.Set("token", f.token)
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("v1", "search")
	u.RawQuery = q.Encode()
	var resp paginatedResponse[searchResultResponse]
	if err := c.get(u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
