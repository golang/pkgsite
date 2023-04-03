// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/sync/errgroup"
	"golang.org/x/vuln/osv"
)

// client is a client for the v1 vulnerability database.
type client struct {
	src source
}

// ByPackage returns the OSV entries matching the package request.
func (c *client) ByPackage(ctx context.Context, req *PackageRequest) (_ []*osv.Entry, err error) {
	derrors.Wrap(&err, "ByPackage(%v)", req)

	b, err := c.modules(ctx)
	if err != nil {
		return nil, err
	}

	dec, err := newStreamDecoder(b)
	if err != nil {
		return nil, err
	}

	var ids []string
	for dec.More() {
		var m ModuleMeta
		err := dec.Decode(&m)
		if err != nil {
			return nil, err
		}
		if m.Path == req.Module {
			for _, v := range m.Vulns {
				// We need to download the full entry if there is no fix,
				// or the requested version is less than the vuln's
				// highest fixed version.
				if v.Fixed == "" || less(req.Version, v.Fixed) {
					ids = append(ids, v.ID)
				}
			}
			// We found the requested module, so skip the rest.
			break
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}

	// Fetch all the entries in parallel, and create a slice
	// containing all the actually affected entries.
	g, gctx := errgroup.WithContext(ctx)
	var mux sync.Mutex
	g.SetLimit(10)
	entries := make([]*osv.Entry, 0, len(ids))
	for _, id := range ids {
		id := id
		g.Go(func() error {
			entry, err := c.ByID(gctx, id)
			if err != nil {
				return err
			}

			if entry == nil {
				return fmt.Errorf("vulnerability %s was found in %s but could not be retrieved", id, modulesEndpoint)
			}

			if isAffected(entry, req) {
				mux.Lock()
				entries = append(entries, entry)
				mux.Unlock()
			}

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})

	return entries, nil
}

// less returns whether v1 < v2, where v1 and v2 are
// semver versions with either a "v", "go" or no prefix.
func less(v1, v2 string) bool {
	return semver.Compare(canonicalizeSemver(v1), canonicalizeSemver(v2)) < 0
}

// canonicalizeSemver turns a SEMVER string into the canonical
// representation using the 'v' prefix as used by the "semver" package.
// Input may be a bare SEMVER ("1.2.3"), Go prefixed SEMVER ("go1.2.3"),
// or already canonical SEMVER ("v1.2.3").
func canonicalizeSemver(s string) string {
	// Remove "go" prefix if needed.
	s = strings.TrimPrefix(s, "go")
	// Add "v" prefix if needed.
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	return s
}

// ByID returns the OSV entry with the given ID or (nil, nil)
// if there isn't one.
func (c *client) ByID(ctx context.Context, id string) (_ *osv.Entry, err error) {
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

// ByAlias returns the OSV entries that have the given alias, or (nil, nil)
// if there are none.
// It returns a list for compatibility with the legacy implementation,
// but the list always contains at most one element.
func (c *client) ByAlias(ctx context.Context, alias string) (_ []*osv.Entry, err error) {
	derrors.Wrap(&err, "ByAlias(%s)", alias)

	b, err := c.vulns(ctx)
	if err != nil {
		return nil, err
	}

	dec, err := newStreamDecoder(b)
	if err != nil {
		return nil, err
	}

	var id string
	for dec.More() {
		var v VulnMeta
		err := dec.Decode(&v)
		if err != nil {
			return nil, err
		}
		for _, vAlias := range v.Aliases {
			if alias == vAlias {
				id = v.ID
				break
			}
		}
		if id != "" {
			break
		}
	}

	if id == "" {
		return nil, nil
	}

	entry, err := c.ByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if entry == nil {
		return nil, fmt.Errorf("vulnerability %s was found in %s but could not be retrieved", id, vulnsEndpoint)
	}

	return []*osv.Entry{entry}, nil
}

// IDs returns a list of the IDs of all the entries in the database.
func (c *client) IDs(ctx context.Context) (_ []string, err error) {
	derrors.Wrap(&err, "IDs()")

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

func (c *client) modules(ctx context.Context) ([]byte, error) {
	return c.src.get(ctx, modulesEndpoint)
}

func (c *client) vulns(ctx context.Context) ([]byte, error) {
	return c.src.get(ctx, vulnsEndpoint)
}

func (c *client) entry(ctx context.Context, id string) ([]byte, error) {
	return c.src.get(ctx, filepath.Join(idDir, id))
}
