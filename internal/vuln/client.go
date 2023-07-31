// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/sync/errgroup"
)

// Client reads Go vulnerability databases.
type Client struct {
	src source
}

// NewClient returns a client that can read from the vulnerability
// database in src (a URL representing either a http or file source).
func NewClient(src string) (*Client, error) {
	s, err := NewSource(src)
	if err != nil {
		return nil, err
	}

	return &Client{src: s}, nil
}

// NewInMemoryClient creates an in-memory vulnerability client for use
// in tests.
func NewInMemoryClient(entries []*osv.Entry) (*Client, error) {
	inMemory, err := newInMemorySource(entries)
	if err != nil {
		return nil, err
	}
	return &Client{src: inMemory}, nil
}

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

func (c *Client) modulesFilter(ctx context.Context, filter func(*ModuleMeta) bool, n int) ([]*ModuleMeta, error) {
	if n == 0 {
		return nil, nil
	}

	b, err := c.modules(ctx)
	if err != nil {
		return nil, err
	}

	dec, err := newStreamDecoder(b)
	if err != nil {
		return nil, err
	}

	ms := make([]*ModuleMeta, 0)
	for dec.More() {
		var m ModuleMeta
		err := dec.Decode(&m)
		if err != nil {
			return nil, err
		}
		if filter(&m) {
			ms = append(ms, &m)
			if len(ms) == n {
				return ms, nil
			}
		}
	}

	if len(ms) == 0 {
		return nil, nil
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

	b, err := c.entry(ctx, id)
	if err != nil {
		// entry only fails if the entry is not found, so do not return
		// the error.
		return nil, nil
	}

	var entry osv.Entry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

// ByAlias returns the Go ID of the OSV entry that has the given alias,
// or a NotFound error if there isn't one.
func (c *Client) ByAlias(ctx context.Context, alias string) (_ string, err error) {
	derrors.Wrap(&err, "ByAlias(%s)", alias)

	b, err := c.vulns(ctx)
	if err != nil {
		return "", err
	}

	dec, err := newStreamDecoder(b)
	if err != nil {
		return "", err
	}

	for dec.More() {
		var v VulnMeta
		err := dec.Decode(&v)
		if err != nil {
			return "", err
		}
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

	return c.byIDs(ctx, ids)
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
	ids = slicesCompact(ids)

	return c.byIDsFilter(ctx, ids, entryMatch)
}

// slicesCompact is a copy of slices.Compact.
// TODO: remove this function and replace its usage with
// slices.Compact once we can depend on it being present
// in the standard library of the previous two Go versions.
func slicesCompact[S ~[]E, E comparable](s S) S {
	if len(s) < 2 {
		return s
	}
	i := 1
	for k := 1; k < len(s); k++ {
		if s[k] != s[k-1] {
			if i != k {
				s[i] = s[k]
			}
			i++
		}
	}
	return s[:i]
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
	b, err := c.vulns(ctx)
	if err != nil {
		return nil, err
	}

	dec, err := newStreamDecoder(b)
	if err != nil {
		return nil, err
	}

	var ids []string
	for dec.More() {
		var v VulnMeta
		err := dec.Decode(&v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, v.ID)
	}

	return ids, nil
}

// newStreamDecoder returns a decoder that can be used
// to read an array of JSON objects.
func newStreamDecoder(b []byte) (*json.Decoder, error) {
	dec := json.NewDecoder(bytes.NewBuffer(b))

	// skip open bracket
	_, err := dec.Token()
	if err != nil {
		return nil, err
	}

	return dec, nil
}

func (c *Client) modules(ctx context.Context) ([]byte, error) {
	return c.src.get(ctx, modulesEndpoint)
}

func (c *Client) vulns(ctx context.Context) ([]byte, error) {
	return c.src.get(ctx, vulnsEndpoint)
}

func (c *Client) entry(ctx context.Context, id string) ([]byte, error) {
	return c.src.get(ctx, path.Join(idDir, id))
}
