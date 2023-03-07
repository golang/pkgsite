// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"
	"errors"

	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

func NewTestClient(entries []*osv.Entry) *Client {
	c := &vulndbTestClient{
		entries:    entries,
		aliasToIDs: map[string][]string{},
	}
	for _, e := range entries {
		for _, a := range e.Aliases {
			c.aliasToIDs[a] = append(c.aliasToIDs[a], e.ID)
		}
	}
	return &Client{c: c}
}

type vulndbTestClient struct {
	vulnc.Client
	entries    []*osv.Entry
	aliasToIDs map[string][]string
}

func (c *vulndbTestClient) GetByModule(context.Context, string) ([]*osv.Entry, error) {
	return nil, errors.New("unimplemented")
}

func (c *vulndbTestClient) GetByID(_ context.Context, id string) (*osv.Entry, error) {
	for _, e := range c.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (c *vulndbTestClient) ListIDs(context.Context) ([]string, error) {
	var ids []string
	for _, e := range c.entries {
		ids = append(ids, e.ID)
	}
	return ids, nil
}

func (c *vulndbTestClient) GetByAlias(ctx context.Context, alias string) ([]*osv.Entry, error) {
	ids := c.aliasToIDs[alias]
	if len(ids) == 0 {
		return nil, nil
	}
	var es []*osv.Entry
	for _, id := range ids {
		e, _ := c.GetByID(ctx, id)
		es = append(es, e)
	}
	return es, nil
}
