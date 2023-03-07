// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"

	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

// Client reads Go vulnerability databases.
type Client struct {
	c vulnc.Client
}

// NewClient returns a client that can read from the vulnerability
// database in src (a URL or local directory).
func NewClient(src string) (*Client, error) {
	c, err := vulnc.NewClient([]string{src}, vulnc.Options{
		HTTPCache: newCache(),
	})
	if err != nil {
		return nil, err
	}
	return &Client{c: c}, nil
}

// ByModule returns the OSV entries that affect the given module path.
func (c *Client) ByModule(ctx context.Context, modulePath string) ([]*osv.Entry, error) {
	return c.c.GetByModule(ctx, modulePath)
}

// ByID returns the OSV entry with the given ID.
func (c *Client) ByID(ctx context.Context, id string) (*osv.Entry, error) {
	return c.c.GetByID(ctx, id)
}

// ByAlias returns the OSV entries that have the given alias.
func (c *Client) ByAlias(ctx context.Context, alias string) ([]*osv.Entry, error) {
	return c.c.GetByAlias(ctx, alias)
}

// IDs returns the IDs of all the entries in the database.
func (c *Client) IDs(ctx context.Context) ([]string, error) {
	return c.c.ListIDs(ctx)
}
