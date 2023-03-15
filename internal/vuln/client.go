// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"
	"fmt"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

// Client reads Go vulnerability databases from both the legacy and v1
// schemas.
//
// If the v1 experiment is active, the client will read from the v1
// database, and will otherwise read from the legacy database.
type Client struct {
	legacy *legacyClient
	v1     *client
}

// NewClient returns a client that can read from the vulnerability
// database in src (a URL representing either a http or file source).
func NewClient(src string) (*Client, error) {
	// Create the v1 client.
	var v1 *client
	s, err := NewSource(src)
	if err != nil {
		// While the v1 client is in experimental mode, ignore the error
		// and always fall back to the legacy client.
		// (An error will occur when using the client if the experiment
		// is enabled and the v1 client is nil).
		v1 = nil
	} else {
		v1 = &client{src: s}
	}

	// Create the legacy client.
	legacy, err := vulnc.NewClient([]string{src}, vulnc.Options{
		HTTPCache: newCache(),
	})
	if err != nil {
		return nil, err
	}

	return &Client{legacy: &legacyClient{legacy}, v1: v1}, nil
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

func (c *Client) ByPackage(ctx context.Context, req *PackageRequest) (_ []*osv.Entry, err error) {
	cli, err := c.cli(ctx)
	if err != nil {
		return nil, err
	}
	return cli.ByPackage(ctx, req)
}

func (c *Client) ByID(ctx context.Context, id string) (*osv.Entry, error) {
	cli, err := c.cli(ctx)
	if err != nil {
		return nil, err
	}
	return cli.ByID(ctx, id)
}

func (c *Client) ByAlias(ctx context.Context, alias string) ([]*osv.Entry, error) {
	cli, err := c.cli(ctx)
	if err != nil {
		return nil, err
	}
	return cli.ByAlias(ctx, alias)
}

func (c *Client) IDs(ctx context.Context) ([]string, error) {
	cli, err := c.cli(ctx)
	if err != nil {
		return nil, err
	}
	return cli.IDs(ctx)
}

// cli returns the underlying client.
// If the v1 experiment is active, it attempts to reurn the v1 client,
// falling back on the legacy client if not set.
// Otherwise, it always returns the legacy client.
func (c *Client) cli(ctx context.Context) (_ cli, err error) {
	derrors.Wrap(&err, "Client.cli()")

	if experiment.IsActive(ctx, internal.ExperimentVulndbV1) {
		if c.v1 == nil {
			return nil, fmt.Errorf("v1 experiment is set, but v1 client is nil")
		}
		return c.v1, nil
	}

	if c.legacy == nil {
		return nil, fmt.Errorf("legacy vulndb client is nil")
	}

	return c.legacy, nil
}

// cli is an interface used temporarily to allow us to support
// both the legacy and v1 databases. It will be removed once we have
// confidence that the v1 cli is working.
type cli interface {
	ByPackage(ctx context.Context, req *PackageRequest) (_ []*osv.Entry, err error)
	ByID(ctx context.Context, id string) (*osv.Entry, error)
	ByAlias(ctx context.Context, alias string) ([]*osv.Entry, error)
	IDs(ctx context.Context) ([]string, error)
}
