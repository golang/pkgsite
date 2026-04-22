// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

//go:generate go test -run=TestTypesUpToDate -update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client fetches data from the pkg.go.dev v1 API.
type Client struct {
	server     *url.URL
	httpClient *http.Client
}

// New creates a new Client.
func New(server string) (*Client, error) {
	u, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	return &Client{
		server:     u,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Items returns an iterator that yields items from a paginated API up to the limit.
// It handles fetching subsequent pages using the token returned by the fetch function.
// The fetch function takes the next page token and the remaining limit.
func Items[T any](startToken string, limit int, fetch func(token string, limit int) (*PaginatedResponse[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		token := startToken
		count := 0
		for {
			reqLimit := 0
			if limit > 0 {
				reqLimit = limit - count
			}
			resp, err := fetch(token, reqLimit)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range resp.Items {
				if !yield(item, nil) {
					return
				}
				count++
				if limit > 0 && count >= limit {
					return
				}
			}
			token = resp.NextPageToken
			if token == "" {
				return
			}
		}
	}
}

// AllItems fetches all pages (or up to limit) and returns the aggregated items and total.
func AllItems[T any](startToken string, limit int, fetch func(token string, limit int) (*PaginatedResponse[T], error)) ([]T, int, error) {
	resp, err := fetch(startToken, limit)
	if err != nil {
		return nil, 0, err
	}
	allItems := resp.Items
	total := resp.Total

	if limit > 0 && len(allItems) >= limit {
		return allItems[:limit], total, nil
	}

	if resp.NextPageToken != "" {
		rem := 0
		if limit > 0 {
			rem = limit - len(allItems)
		}
		for item, err := range Items(resp.NextPageToken, rem, fetch) {
			if err != nil {
				// TODO(hyangah): consider to return allItems accumulated so far instead of throwing away.
				return nil, 0, err
			}
			allItems = append(allItems, item)
		}
	}
	return allItems, total, nil
}

func (e *Error) Error() string {
	if len(e.Candidates) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s; specify module path:\n", e.Message)
		for _, c := range e.Candidates {
			fmt.Fprintf(&b, "  --module=%s\n", c.ModulePath)
		}
		return b.String()
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Code)
}

// get fetches url and decodes the JSON response into dst.
func (c *Client) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		var aerr Error
		if json.Unmarshal(body, &aerr) == nil && aerr.Message != "" {
			return &aerr
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// PackageOptions contains options for GetPackage.
type PackageOptions struct {
	Module   string
	Doc      string
	Examples bool
	Imports  bool
	Licenses bool
	GOOS     string
	GOARCH   string
}

func (c *Client) GetPackage(ctx context.Context, path, version string, opts PackageOptions) (*Package, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Module != "" {
		q.Set("module", opts.Module)
	}
	if opts.Doc != "" {
		q.Set("doc", opts.Doc)
	}
	if opts.Examples {
		q.Set("examples", "true")
	}
	if opts.Imports {
		q.Set("imports", "true")
	}
	if opts.Licenses {
		q.Set("licenses", "true")
	}
	if opts.GOOS != "" {
		q.Set("goos", opts.GOOS)
	}
	if opts.GOARCH != "" {
		q.Set("goarch", opts.GOARCH)
	}
	u := c.server.JoinPath("v1", "package", path)
	u.RawQuery = q.Encode()

	var resp Package
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PaginationOptions contains common pagination options.
type PaginationOptions struct {
	Limit int
	Token string
}

// SymbolsOptions contains options for GetSymbols.
type SymbolsOptions struct {
	Module string
	GOOS   string
	GOARCH string
	PaginationOptions
}

func (c *Client) GetSymbols(ctx context.Context, path, version string, opts SymbolsOptions) (*PaginatedResponse[Symbol], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Module != "" {
		q.Set("module", opts.Module)
	}
	if opts.GOOS != "" {
		q.Set("goos", opts.GOOS)
	}
	if opts.GOARCH != "" {
		q.Set("goarch", opts.GOARCH)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "symbols", path)
	u.RawQuery = q.Encode()
	var resp PackageSymbols
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp.Symbols, nil
}

// ImportedByOptions contains options for GetImportedBy.
type ImportedByOptions struct {
	Module string
	PaginationOptions
}

func (c *Client) GetImportedBy(ctx context.Context, path, version string, opts ImportedByOptions) (*PackageImportedBy, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Module != "" {
		q.Set("module", opts.Module)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "imported-by", path)
	u.RawQuery = q.Encode()
	var resp PackageImportedBy
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ModuleOptions contains options for GetModule.
type ModuleOptions struct {
	Readme   bool
	Licenses bool
}

func (c *Client) GetModule(ctx context.Context, path, version string, opts ModuleOptions) (*Module, error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Readme {
		q.Set("readme", "true")
	}
	if opts.Licenses {
		q.Set("licenses", "true")
	}
	u := c.server.JoinPath("v1", "module", path)
	u.RawQuery = q.Encode()
	var resp Module
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VersionResponse is a single version from /v1/versions/.
type VersionResponse struct {
	Version string `json:"version"`
}

func (c *Client) GetVersions(ctx context.Context, path string, opts PaginationOptions) (*PaginatedResponse[VersionResponse], error) {
	q := make(url.Values)
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "versions", path)
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[VersionResponse]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetVulns(ctx context.Context, path, version string, opts PaginationOptions) (*PaginatedResponse[Vulnerability], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "vulns", path)
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[Vulnerability]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ModulePackageResponse is a single package from /v1/packages/.
type ModulePackageResponse struct {
	Path     string `json:"path"`
	Synopsis string `json:"synopsis"`
}

func (c *Client) GetPackages(ctx context.Context, modulePath, version string, opts PaginationOptions) (*PaginatedResponse[ModulePackageResponse], error) {
	q := make(url.Values)
	if version != "" {
		q.Set("version", version)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "packages", modulePath)
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[ModulePackageResponse]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchOptions contains options for Search.
type SearchOptions struct {
	Symbol string
	PaginationOptions
}

func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (*PaginatedResponse[SearchResult], error) {
	q := make(url.Values)
	q.Set("q", query)
	if opts.Symbol != "" {
		q.Set("symbol", opts.Symbol)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1", "search")
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[SearchResult]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
