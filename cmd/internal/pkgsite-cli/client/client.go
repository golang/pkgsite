// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package client provides a client for the pkg.go.dev v1beta API.
package client

//go:generate go test -run=TestTypesUpToDate -update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client fetches data from the pkg.go.dev v1beta API.
type Client struct {
	server     *url.URL
	httpClient *http.Client
	// If true, print every URL fetched to Output.
	PrintURLs bool
	// Where to write URLs and other debug information.
	Output io.Writer
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
		Output:     os.Stderr,
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

// Is429 returns true if err is an HTTP 429 (Too Many Requests) error.
func Is429(err error) bool {
	var aerr *Error
	if errors.As(err, &aerr) {
		return aerr.Code == http.StatusTooManyRequests
	}
	return false
}

// AllItems fetches all pages (or up to limit) and returns the aggregated items, total, next page token (if interrupted), and error.
func AllItems[T any](startToken string, limit int, fetch func(token string, limit int) (*PaginatedResponse[T], error)) (_ []T, total int, token string, _ error) {
	token = startToken
	var allItems []T

	for {
		reqLimit := 0
		if limit > 0 {
			reqLimit = limit - len(allItems)
			if reqLimit <= 0 {
				return allItems, total, token, nil
			}
		}

		resp, err := fetch(token, reqLimit)
		if err != nil {
			// No matter what went wrong, return partial results so the
			// user can continue.
			return allItems, total, token, err
		}

		itemsToAppend := resp.Items
		hitLimit := false
		if limit > 0 && len(allItems)+len(itemsToAppend) >= limit {
			allowed := limit - len(allItems)
			itemsToAppend = itemsToAppend[:allowed]
			hitLimit = true
		}

		allItems = append(allItems, itemsToAppend...)
		total = resp.Total

		if hitLimit {
			nextToken := resp.NextPageToken
			if len(resp.Items) > len(itemsToAppend) {
				if token != startToken {
					// We dropped items from the current page because we hit
					// a limit, and we've advanced at least one page since
					// we started. That means if we restart with this page,
					// we may get past the limit the next time.
					// TODO: include the position in the page with the token,
					// so we can restart exactly where we left off.
					nextToken = token
				} else {
					// The page we started at: we'll never advance, so give up.
					nextToken = ""
				}
			}
			return allItems, total, nextToken, nil
		}

		token = resp.NextPageToken
		if token == "" {
			break
		}
	}
	return allItems, total, "", nil
}

func (e *Error) Error() string {
	if len(e.Candidates) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s; specify module path:\n", e.Message)
		for _, c := range e.Candidates {
			fmt.Fprintf(&b, "  -module=%s\n", c.ModulePath)
		}
		return b.String()
	}

	status := ""
	if e.Code >= 100 {
		status = fmt.Sprintf(" (HTTP %d)", e.Code)
	}

	if len(e.Fixes) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s%s\nTo fix:\n", e.Message, status)
		for _, f := range e.Fixes {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		return b.String()
	}
	return fmt.Sprintf("%s%s", e.Message, status)
}

// get fetches url and decodes the JSON response into dst.
func (c *Client) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pkgsite-cli/v1")

	if c.PrintURLs {
		fmt.Fprintln(c.Output, url)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit to 1MB
		if err == nil {
			var aerr Error
			if json.Unmarshal(body, &aerr) == nil && aerr.Message != "" {
				aerr.Code = resp.StatusCode
				return &aerr
			}
		}
		// The body can't be read, but still return an Error
		// with the information we have.
		return &Error{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
		}
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

// GetPackage fetches package information for the given path and version.
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
	u := c.server.JoinPath("v1beta", "package", path)
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

// GetSymbols fetches symbols for the given package path and version.
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
	u := c.server.JoinPath("v1beta", "symbols", path)
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

// GetImportedBy fetches packages that import the given package path and version.
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
	u := c.server.JoinPath("v1beta", "imported-by", path)
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

// GetModule fetches module information for the given path and version.
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
	u := c.server.JoinPath("v1beta", "module", path)
	u.RawQuery = q.Encode()
	var resp Module
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VersionResponse is a single version from /v1beta/versions/.
type VersionResponse struct {
	Version string `json:"version"`
}

// GetVersions fetches a list of versions for the given module path.
func (c *Client) GetVersions(ctx context.Context, path string, opts PaginationOptions) (*PaginatedResponse[VersionResponse], error) {
	q := make(url.Values)
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	u := c.server.JoinPath("v1beta", "versions", path)
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[VersionResponse]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetVulns fetches a list of vulnerabilities for the given module path and version.
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
	u := c.server.JoinPath("v1beta", "vulns", path)
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[Vulnerability]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ModulePackageResponse is a single package from /v1beta/packages/.
type ModulePackageResponse struct {
	Path     string `json:"path"`
	Synopsis string `json:"synopsis"`
}

// GetPackages fetches a list of packages for the given module path and version.
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
	u := c.server.JoinPath("v1beta", "packages", modulePath)
	u.RawQuery = q.Encode()
	var resp PackagesResponse
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	var items []ModulePackageResponse
	for _, p := range resp.Packages.Items {
		items = append(items, ModulePackageResponse{
			Path:     p.Path,
			Synopsis: p.Synopsis,
		})
	}
	return &PaginatedResponse[ModulePackageResponse]{
		Items:         items,
		Total:         resp.Packages.Total,
		NextPageToken: resp.Packages.NextPageToken,
	}, nil
}

// SearchOptions contains options for Search.
type SearchOptions struct {
	Symbol string
	PaginationOptions
}

// Search queries the pkg.go.dev API for packages matching the given query.
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
	u := c.server.JoinPath("v1beta", "search")
	u.RawQuery = q.Encode()
	var resp PaginatedResponse[SearchResult]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
