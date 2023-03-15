// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"
	"fmt"

	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

// Client reads Go vulnerability databases.
type Client struct {
	legacy *legacyClient
	// v1 client, currently for testing only.
	// Always nil if created via NewClient.
	v1 *client
}

// NewClient returns a client that can read from the vulnerability
// database in src (a URL representing either a http or file source).
func NewClient(src string) (*Client, error) {
	legacy, err := vulnc.NewClient([]string{src}, vulnc.Options{
		HTTPCache: newCache(),
	})
	if err != nil {
		return nil, err
	}

	return &Client{legacy: &legacyClient{legacy}}, nil
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

// cli returns the underlying client, favoring the legacy client
// if both are present.
func (c *Client) cli(ctx context.Context) (cli, error) {
	if c.legacy != nil {
		return c.legacy, nil
	}
	if c.v1 != nil {
		return c.v1, nil
	}
	return nil, fmt.Errorf("vuln.Client: no underlying client defined")
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
