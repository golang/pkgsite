// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"errors"
	"fmt"
	"reflect"
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
	{ID: "GO-1990-01", Details: "a"},
	{ID: "GO-1990-02", Details: "b"},
	{ID: "GO-1990-10", Details: "c"},
	{ID: "GO-1991-01", Details: "d"},
	{ID: "GO-1991-05", Details: "e"},
	{ID: "GO-1991-23", Details: "f"},
	{ID: "GO-1991-30", Details: "g"},
}

func TestNewVulnListPage(t *testing.T) {
	c := &vulndbTestClient{entries: testEntries}
	got, err := newVulnListPage(c)
	if err != nil {
		t.Fatal(err)
	}
	// testEntries is already sorted by ID, but it should be reversed.
	var wantEntries []*osv.Entry
	for i := len(testEntries) - 1; i >= 0; i-- {
		wantEntries = append(wantEntries, testEntries[i])
	}
	want := &VulnListPage{Entries: wantEntries}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(VulnListPage{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestNewVulnPage(t *testing.T) {
	c := &vulndbTestClient{entries: testEntries}
	got, err := newVulnPage(c, "GO-1990-02")
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

func TestCollectRangePairs(t *testing.T) {
	in := osv.Affected{
		Package: osv.Package{Name: "github.com/a/b"},
		Ranges: osv.Affects{
			{Type: osv.TypeSemver, Events: []osv.RangeEvent{{Introduced: "", Fixed: "0.5"}}},
			{Type: osv.TypeSemver, Events: []osv.RangeEvent{
				{Introduced: "1.2"}, {Fixed: "1.5"},
				{Introduced: "2.1", Fixed: "2.3"},
			}},
			{Type: osv.TypeGit, Events: []osv.RangeEvent{{Introduced: "a", Fixed: "b"}}},
		},
	}
	got := collectRangePairs(in)
	want := []pair{
		{"", "v0.5"},
		{"v1.2", "v1.5"},
		{"v2.1", "v2.3"},
		{"a", "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
	}

}

func Test_aliasLinks(t *testing.T) {
	type args struct {
		e *osv.Entry
	}
	tests := []struct {
		name string
		args args
		want []link
	}{
		{
			"reserved cve",
			args{&osv.Entry{Aliases: []string{"CVE-0000-00000"}}},
			[]link{{Body: "CVE-0000-00000"}},
		},
		{
			"nist",
			args{&osv.Entry{Aliases: []string{"CVE-0000-00000"}, References: []osv.Reference{{Type: "ADVISORY", URL: nistAdvisoryUrlPrefix + "CVE-0000-00000"}}}},
			[]link{{Body: "CVE-0000-00000", Href: nistAdvisoryUrlPrefix + "CVE-0000-00000"}},
		},
		{
			"github",
			args{&osv.Entry{Aliases: []string{"GHSA-zz00-zzz0-0zz0"}}},
			[]link{{Body: "GHSA-zz00-zzz0-0zz0", Href: githubAdvisoryUrlPrefix + "GHSA-zz00-zzz0-0zz0"}},
		},
		{
			"empty link",
			args{&osv.Entry{Aliases: []string{"NA-0000"}}},
			[]link{{Body: "NA-0000"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aliasLinks(tt.args.e)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch(-want, +got): %s", diff)
			}
		})
	}
}
