// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/sync/errgroup"
)

// Client reads Go vulnerability databases.
type Client struct {
	src             source
	mu              sync.Mutex
	cache           map[string]any
	modified        time.Time // the modified time of the DB
	modifiedFetched time.Time // when we last read the modified time from the source
}

// NewClient returns a client that can read from the vulnerability
// database in src, a URL representing either an http or file source.
func NewClient(src string) (*Client, error) {
	s, err := NewSource(src)
	if err != nil {
		return nil, err
	}
	return newClient(s), nil
}

// NewInMemoryClient creates an in-memory vulnerability client for use
// in tests.
func NewInMemoryClient(entries []*osv.Entry) (*Client, error) {
	inMemory, err := newInMemorySource(entries)
	if err != nil {
		return nil, err
	}
	return newClient(inMemory), nil
}

func newClient(src source) *Client {
	return &Client{src: src, cache: map[string]any{}}
}

// A PackageRequest provides arguments to [Client.ByPackage].
type PackageRequest struct {
	// Module is the module path to filter on.
	// ByPackage will only return entries that affect this module.
	// This must be set (if empty, ByPackage will always return nil).
	Module string
	// The package path to filter on.
	// ByPackage will only return entries that affect this package.
	// If empty, ByPackage will not filter based on the package.
	Package string
	// The version to filter on.
	// ByPackage will only return entries affected at this module
	// version.
	// If empty, ByPackage will not filter based on version.
	Version string
}

// ByPackage returns the OSV entries matching the package request.
func (c *Client) ByPackage(ctx context.Context, req *PackageRequest) (_ []*osv.Entry, err error) {
	derrors.Wrap(&err, "ByPackage(%v)", req)

	// Find the metadata for the module with the given module path.
	ms, err := c.modulesFilter(ctx, func(m *ModuleMeta) bool {
		return m.Path == req.Module
	}, 1)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, nil
	}

	// Figure out which vulns we actually need to download.
	var ids []string
	for _, v := range ms[0].Vulns {
		// We need to download the full entry if there is no fix,
		// or the requested version is less than the vuln's
		// highest fixed version.
		if v.Fixed == "" || osv.LessSemver(req.Version, v.Fixed) {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	return c.byIDsFilter(ctx, ids, func(e *osv.Entry) bool {
		return isAffected(e, req)
	})
}

// modulesFilter returns all the modules in the DB for which filter returns true.
func (c *Client) modulesFilter(ctx context.Context, filter func(*ModuleMeta) bool, n int) ([]*ModuleMeta, error) {
	if n == 0 {
		return nil, nil
	}
	all, _, err := get[[]*ModuleMeta](ctx, c, modulesEndpoint)
	if err != nil {
		return nil, err
	}

	var ms []*ModuleMeta
	for _, m := range all {
		if filter(m) {
			ms = append(ms, m)
		}
	}
	return ms, nil
}

func isAffected(e *osv.Entry, req *PackageRequest) bool {
	for _, a := range e.Affected {
		if a.Module.Path != req.Module || !osv.AffectsSemver(a.Ranges, req.Version) {
			continue
		}
		if packageMatches := func() bool {
			if req.Package == "" {
				return true //  match module only
			}
			if len(a.EcosystemSpecific.Packages) == 0 {
				return true // no package info available, so match on module
			}
			for _, p := range a.EcosystemSpecific.Packages {
				if req.Package == p.Path {
					return true // package matches
				}
			}
			return false
		}(); !packageMatches {
			continue
		}
		return true
	}
	return false
}

// ByID returns the OSV entry with the given ID or (nil, nil)
// if there isn't one.
func (c *Client) ByID(ctx context.Context, id string) (_ *osv.Entry, err error) {
	derrors.Wrap(&err, "ByID(%s)", id)

	entry, _, err := get[osv.Entry](ctx, c, path.Join(idDir, id))
	if err != nil {
		// entry only fails if the entry is not found, so do not return
		// the error.
		return nil, nil
	}
	return &entry, nil
}

// ByAlias returns the Go ID of the OSV entry that has the given alias,
// or a NotFound error if there isn't one.
func (c *Client) ByAlias(ctx context.Context, alias string) (_ string, err error) {
	derrors.Wrap(&err, "ByAlias(%s)", alias)

	vs, err := c.vulns(ctx)
	if err != nil {
		return "", err
	}
	for _, v := range vs {
		for _, vAlias := range v.Aliases {
			if alias == vAlias {
				return v.ID, nil
			}
		}
	}
	return "", derrors.NotFound
}

// Entries returns all entries in the database, sorted in descending
// order by Go ID (most recent to least recent).
// If n >= 0, only the n most recent entries are returned.
func (c *Client) Entries(ctx context.Context, n int) (_ []*osv.Entry, err error) {
	derrors.Wrap(&err, "Entries(n=%d)", n)

	if n == 0 {
		return nil, nil
	}

	ids, err := c.IDs(ctx)
	if err != nil {
		return nil, err
	}
	sortIDs(ids)

	if n >= 0 && len(ids) > n {
		ids = ids[:n]
	}

	entries, err := c.byIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	// We've seen nil entries crash the vuln/list page.
	// Add logging to understand why.
	for i, e := range entries {
		if e == nil {
			log.Errorf(ctx, "Client.Entries: got nil osv.Entry for ID %q", ids[i])
		}
	}
	return entries, nil
}

func sortIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

}

// ByPackagePrefix returns all the OSV entries that match the given
// package prefix, in descending order by ID, or (nil, nil) if there
// are none.
//
// An entry matches a prefix if:
//   - Any affected module or package equals the given prefix, OR
//   - Any affected module or package's path begins with the given prefix
//     interpreted as a full path. (E.g. "example.com/module/package" matches
//     the prefix "example.com/module" but not "example.com/mod")
func (c *Client) ByPackagePrefix(ctx context.Context, prefix string) (_ []*osv.Entry, err error) {
	derrors.Wrap(&err, "ByPackagePrefix(%s)", prefix)

	prefix = strings.TrimSuffix(prefix, "/")
	prefixPath := prefix + "/"
	prefixMatch := func(s string) bool {
		return s == prefix || strings.HasPrefix(s, prefixPath)
	}

	moduleMatch := func(m *ModuleMeta) bool {
		// If the prefix possibly refers to a standard library package,
		// always look at the stdlib and toolchain modules.
		if stdlib.Contains(prefix) &&
			(m.Path == osv.GoStdModulePath || m.Path == osv.GoCmdModulePath) {
			return true
		}
		// Look at the module if it is either prefixed by the prefix,
		// or it is itself a prefix of the prefix.
		// (The latter case catches queries that are prefixes of the package
		// path but longer than the module path).
		return prefixMatch(m.Path) || strings.HasPrefix(prefix, m.Path)
	}

	entryMatch := func(e *osv.Entry) bool {
		for _, aff := range e.Affected {
			if prefixMatch(aff.Module.Path) {
				return true
			}
			for _, pkg := range aff.EcosystemSpecific.Packages {
				if prefixMatch(pkg.Path) {
					return true
				}
			}
		}
		return false
	}

	ms, err := c.modulesFilter(ctx, moduleMatch, -1)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, nil
	}

	var ids []string
	for _, m := range ms {
		for _, vs := range m.Vulns {
			ids = append(ids, vs.ID)
		}
	}
	sortIDs(ids)
	// Remove any duplicates.
	ids = slices.Compact(ids)

	return c.byIDsFilter(ctx, ids, entryMatch)
}

func (c *Client) byIDsFilter(ctx context.Context, ids []string, filter func(*osv.Entry) bool) (_ []*osv.Entry, err error) {
	entries, err := c.byIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var filtered []*osv.Entry
	for _, entry := range entries {
		if entry != nil && filter(entry) {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return filtered, nil
}

// byIDs returns OSV entries for given ids.
// The i-th entry in the returned value corresponds to the i-th ID
// and can be nil if there is no entry with the given ID.
func (c *Client) byIDs(ctx context.Context, ids []string) (_ []*osv.Entry, err error) {
	entries := make([]*osv.Entry, len(ids))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	for i, id := range ids {
		i, id := i, id
		g.Go(func() error {
			e, err := c.ByID(gctx, id)
			if err != nil {
				return err
			}
			entries[i] = e
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return entries, nil
}

// IDs returns a list of the IDs of all the entries in the database.
func (c *Client) IDs(ctx context.Context) (_ []string, err error) {
	vs, err := c.vulns(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, v := range vs {
		ids = append(ids, v.ID)
	}
	return ids, nil
}

func (c *Client) vulns(ctx context.Context) ([]VulnMeta, error) {
	vms, _, err := get[[]VulnMeta](ctx, c, vulnsEndpoint)
	return vms, err
}

// After this time, consider our value of modified to be stale.
// var for testing.
var modifiedStaleDur = 5 * time.Minute

// get returns the contents of endpoint as a T, checking the cache first.
// It also reports whether it found the value in the cache.
func get[T any](ctx context.Context, c *Client, endpoint string) (t T, cached bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.modifiedFetched) > modifiedStaleDur {
		// c.modified is stale; reread the DB's modified time.
		data, err := c.src.get(ctx, dbEndpoint)
		if err != nil {
			return t, false, err
		}
		var m DBMeta
		if err := json.Unmarshal(data, &m); err != nil {
			return t, false, fmt.Errorf("unmarshaling DBMeta: %w", err)
		}
		c.modifiedFetched = time.Now()
		// If the DB has been modified since the last time we checked,
		// clear the cache and note the new modified time.
		// We only compare modified times with each other, as per
		// https://go.dev/doc/security/vuln/database#api:
		// "the modified time should not be compared to wall clock time".
		if !m.Modified.Equal(c.modified) {
			clear(c.cache)
			c.modified = m.Modified
		}
	}
	if mms, ok := c.cache[endpoint]; ok {
		return mms.(T), true, nil
	}
	c.mu.Unlock()
	data, err := c.src.get(ctx, endpoint)
	c.mu.Lock()
	// NOTE: Errors aren't cached, so an endpoint that repeatedly fails will be expensive.
	// On the other hand, we won't turn transient errors into near-permanent ones.
	// TODO: cache 4xx errors, since we get a lot of spammy traffic.
	if err != nil {
		return t, false, err
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, false, err
	}
	c.cache[endpoint] = t
	return t, false, nil
}
