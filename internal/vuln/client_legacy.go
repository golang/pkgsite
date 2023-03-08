// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"

	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

type legacyClient struct {
	vulnc.Client
}

func (c *legacyClient) ByPackage(ctx context.Context, req *PackageRequest) (_ []*osv.Entry, err error) {
	// Get all the vulns for this module.
	moduleEntries, err := c.GetByModule(ctx, req.Module)
	if err != nil {
		return nil, err
	}

	// Filter out entries that do not apply to this package/version.
	var packageEntries []*osv.Entry
	for _, e := range moduleEntries {
		if isAffected(e, req) {
			packageEntries = append(packageEntries, e)
		}
	}

	return packageEntries, nil
}

// ByID returns the OSV entry with the given ID.
func (c *legacyClient) ByID(ctx context.Context, id string) (*osv.Entry, error) {
	return c.GetByID(ctx, id)
}

// ByAlias returns the OSV entries that have the given alias.
func (c *legacyClient) ByAlias(ctx context.Context, alias string) ([]*osv.Entry, error) {
	return c.GetByAlias(ctx, alias)
}

// IDs returns the IDs of all the entries in the database.
func (c *legacyClient) IDs(ctx context.Context) ([]string, error) {
	return c.ListIDs(ctx)
}

func isAffected(e *osv.Entry, req *PackageRequest) bool {
	for _, a := range e.Affected {
		// a.Package.Name is Go "module" name. Go package path is a.EcosystemSpecific.Imports.Path.
		if a.Package.Name != req.Module || !a.Ranges.AffectsSemver(req.Version) {
			continue
		}
		if packageMatches := func() bool {
			if req.Package == "" {
				return true //  match module only
			}
			if len(a.EcosystemSpecific.Imports) == 0 {
				return true // no package info available, so match on module
			}
			for _, p := range a.EcosystemSpecific.Imports {
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
