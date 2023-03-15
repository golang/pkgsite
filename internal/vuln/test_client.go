// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"

	"golang.org/x/tools/txtar"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

// NewTestClient creates an in-memory client for use in tests,
// It's logic is different from the real client, so it should not be used to
// test the client itself, but can be used to test code that depends on the
// client.
func NewTestClient(entries []*osv.Entry) *Client {
	return &Client{legacy: newTestLegacyClient(entries)}
}

// newTestV1Client creates an in-memory client for use in tests.
// It uses all the logic of the real v1 client, except that it reads
// raw database data from the given txtar file instead of making HTTP
// requests.
// It can be used to test core functionality of the v1 client.
func newTestV1Client(txtarFile string) (*client, error) {
	data := make(map[string][]byte)

	ar, err := txtar.ParseFile(txtarFile)
	if err != nil {
		return nil, err
	}

	for _, f := range ar.Files {
		data[f.Name] = f.Data
	}

	return &client{&inMemorySource{data: data}}, nil
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
