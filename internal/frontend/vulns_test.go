// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

func TestVulnsForPackage(t *testing.T) {
	e := osv.Entry{
		Details: "bad",
		Affected: []osv.Affected{{
			Package: osv.Package{Name: "bad.com"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.2.3"}},
			}},
		}},
	}

	get := func(modulePath string) ([]*osv.Entry, error) {
		switch modulePath {
		case "good.com":
			return nil, nil
		case "bad.com":
			return []*osv.Entry{&e}, nil
		default:
			return nil, fmt.Errorf("unknown module %q", modulePath)
		}
	}

	got := VulnsForPackage("good.com", "v1.0.0", "good.com", get)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	got = VulnsForPackage("bad.com", "v1.0.0", "bad.com", get)
	want := []Vuln{{
		Details:      "bad",
		FixedVersion: "v1.2.3",
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}

	got = VulnsForPackage("bad.com", "v1.3.0", "bad.com", get)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

var testEntries = []*osv.Entry{
	{ID: "one", Details: "a"},
	{ID: "two", Details: "b"},
	{ID: "three", Details: "c"},
	{ID: "four", Details: "d"},
	{ID: "five", Details: "e"},
	{ID: "six", Details: "f"},
	{ID: "seven", Details: "g"},
}

func TestNewVulnListPage(t *testing.T) {
	c := &vulndbTestClient{entries: testEntries}
	got, err := newVulnListPage(c)
	if err != nil {
		t.Fatal(err)
	}
	// testEntries is already sorted by ID.
	want := &VulnListPage{Entries: testEntries}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(VulnListPage{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestNewVulnPage(t *testing.T) {
	c := &vulndbTestClient{entries: testEntries}
	got, err := newVulnPage(c, "two")
	if err != nil {
		t.Fatal(err)
	}
	want := &VulnPage{Entry: testEntries[1]}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(VulnPage{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

type vulndbTestClient struct {
	vulnc.Client
	entries []*osv.Entry
}

func (c *vulndbTestClient) GetByModule(string) ([]*osv.Entry, error) {
	return nil, errors.New("unimplemented")
}

func (c *vulndbTestClient) GetByID(id string) (*osv.Entry, error) {
	for _, e := range c.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (c *vulndbTestClient) ListIDs() ([]string, error) {
	var ids []string
	for _, e := range c.entries {
		ids = append(ids, e.ID)
	}
	return ids, nil
}
