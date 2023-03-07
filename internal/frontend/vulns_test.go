// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal/vuln"
	"golang.org/x/vuln/osv"
)

var testEntries = []*osv.Entry{
	{ID: "GO-1990-01", Details: "a", Aliases: []string{"CVE-2000-1", "GHSA-aaaa-bbbb-cccc"}},
	{ID: "GO-1990-02", Details: "b", Aliases: []string{"CVE-2000-1", "GHSA-1111-2222-3333"}},
	{ID: "GO-1990-10", Details: "c"},
	{ID: "GO-1991-01", Details: "d"},
	{ID: "GO-1991-05", Details: "e"},
	{ID: "GO-1991-23", Details: "f"},
	{ID: "GO-1991-30", Details: "g"},
	{
		ID:      "GO-1991-31",
		Details: "h",
		Affected: []osv.Affected{{
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{
					{
						Path: "example.com/org/path",
					},
				},
			},
		}},
	},
}

func TestNewVulnListPage(t *testing.T) {
	ctx := context.Background()
	c := vuln.NewTestClient(testEntries)
	got, err := newVulnListPage(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	// testEntries is already sorted by ID, but it should be reversed.
	var wantEntries []OSVEntry
	for i := len(testEntries) - 1; i >= 0; i-- {
		wantEntries = append(wantEntries, OSVEntry{testEntries[i]})
	}
	want := &VulnListPage{Entries: wantEntries}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(VulnListPage{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestNewVulnPage(t *testing.T) {
	ctx := context.Background()
	c := vuln.NewTestClient(testEntries)
	got, err := newVulnPage(ctx, c, "GO-1990-02")
	if err != nil {
		t.Fatal(err)
	}
	want := &VulnPage{
		Entry:      OSVEntry{testEntries[1]},
		AliasLinks: aliasLinks(testEntries[1]),
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(VulnPage{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
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
			"mitre",
			args{&osv.Entry{Aliases: []string{"CVE-0000-00000"}, References: []osv.Reference{{Type: "ADVISORY", URL: mitreAdvisoryUrlPrefix + "CVE-0000-00000"}}}},
			[]link{{Body: "CVE-0000-00000", Href: mitreAdvisoryUrlPrefix + "CVE-0000-00000"}},
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

func Test_advisoryLinks(t *testing.T) {
	type args struct {
		e *osv.Entry
	}
	tests := []struct {
		name string
		args args
		want []link
	}{
		{
			"nist",
			args{&osv.Entry{Aliases: []string{"CVE-0000-00000"}, References: []osv.Reference{{Type: "ADVISORY", URL: nistAdvisoryUrlPrefix + "CVE-0000-00000"}}}},
			[]link{{Body: nistAdvisoryUrlPrefix + "CVE-0000-00000", Href: nistAdvisoryUrlPrefix + "CVE-0000-00000"}},
		},
		{
			"empty link",
			args{&osv.Entry{Aliases: []string{"NA-0000"}}},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := advisoryLinks(tt.args.e)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch(-want, +got): %s", diff)
			}
		})
	}
}
