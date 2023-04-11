// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"bytes"
	"context"
	"encoding/json"

	"golang.org/x/tools/txtar"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

// NewInMemoryClient creates an in-memory client for use in tests.
func NewInMemoryClient(entries []*osv.Entry) (*Client, error) {
	inMemory, err := newInMemorySource(entries)
	if err != nil {
		return nil, err
	}
	return &Client{legacy: newTestLegacyClient(entries), v1: &client{inMemory}}, nil
}

// newTestClientFromTxtar creates an in-memory client for use in tests.
// It reads test data from a txtar file which must follow the
// v1 database schema.
func newTestClientFromTxtar(txtarFile string) (*client, error) {
	data := make(map[string][]byte)

	ar, err := txtar.ParseFile(txtarFile)
	if err != nil {
		return nil, err
	}

	for _, f := range ar.Files {
		fdata, err := removeWhitespace(f.Data)
		if err != nil {
			return nil, err
		}
		data[f.Name] = fdata
	}

	return &client{&inMemorySource{data: data}}, nil
}

func removeWhitespace(data []byte) ([]byte, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func newTestLegacyClient(entries []*osv.Entry) *legacyClient {
	c := &testVulnClient{
		entries:          entries,
		aliasToIDs:       map[string][]string{},
		modulesToEntries: map[string][]*osv.Entry{},
	}
	for _, e := range entries {
		for _, a := range e.Aliases {
			c.aliasToIDs[a] = append(c.aliasToIDs[a], e.ID)
		}
		for _, affected := range e.Affected {
			c.modulesToEntries[affected.Package.Name] = append(c.modulesToEntries[affected.Package.Name], e)
		}
	}
	return &legacyClient{c}
}

// Implements x/vuln.Client.
type testVulnClient struct {
	vulnc.Client
	entries          []*osv.Entry
	aliasToIDs       map[string][]string
	modulesToEntries map[string][]*osv.Entry
}

func (c *testVulnClient) GetByModule(_ context.Context, module string) ([]*osv.Entry, error) {
	return c.modulesToEntries[module], nil
}

func (c *testVulnClient) GetByID(_ context.Context, id string) (*osv.Entry, error) {
	for _, e := range c.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (c *testVulnClient) ListIDs(context.Context) ([]string, error) {
	var ids []string
	for _, e := range c.entries {
		ids = append(ids, e.ID)
	}
	return ids, nil
}

func (c *testVulnClient) GetByAlias(ctx context.Context, alias string) ([]*osv.Entry, error) {
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
