// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/vuln"
)

var testEntries = []*osv.Entry{
	{ID: "GO-1990-0001", Details: "a", Aliases: []string{"CVE-2000-1", "GHSA-cccc-ffff-gggg"}},
	{ID: "GO-1990-0002", Details: "b", Aliases: []string{"CVE-2000-1", "GHSA-1111-2222-3333"}},
	{ID: "GO-1990-0010", Details: "c"},
	{ID: "GO-1991-0001", Details: "d"},
	{ID: "GO-1991-0005", Details: "e"},
	{ID: "GO-1991-0023", Details: "f",
		Affected: []osv.Affected{{
			Module: osv.Module{
				Path: "stdlib",
			},
			EcosystemSpecific: osv.EcosystemSpecific{
				Packages: []osv.Package{
					{
						Path: "net/http",
					}}}}},
	},
	{ID: "GO-1991-0030", Details: "g",
		Affected: []osv.Affected{{
			Module: osv.Module{
				Path: "example.com/org/repo",
			},
		}}},
	{
		ID:      "GO-1991-0031",
		Details: "h",
		Affected: []osv.Affected{{
			Module: osv.Module{
				Path: "example.com/org/module",
			},
			EcosystemSpecific: osv.EcosystemSpecific{
				Packages: []osv.Package{
					{
						Path: "example.com/org/module/a/package",
					},
				},
			},
		}},
	},
}

func TestNewVulnPage(t *testing.T) {
	ctx := context.Background()
	c, err := vuln.NewInMemoryClient(testEntries)
	if err != nil {
		t.Fatal(err)
	}

	// testEntries is already sorted by ID, but it should be reversed.
	var wantEntries []*osv.Entry
	for i := len(testEntries) - 1; i >= 0; i-- {
		wantEntries = append(wantEntries, testEntries[i])
	}

	tcs := []struct {
		name string
		url  string
		want *vulnPage
	}{
		{
			name: "main vuln page",
			url:  "https://pkg.go.dev/vuln/",
			want: &vulnPage{
				page:     &VulnListPage{Entries: wantEntries[:5]},
				template: "vuln/main",
				title:    "Go Vulnerability Database",
			},
		},
		{
			name: "all vulns page",
			url:  "https://pkg.go.dev/vuln/list",
			want: &vulnPage{
				page:     &VulnListPage{Entries: wantEntries},
				template: "vuln/list",
				title:    "Vulnerability Reports",
			},
		},
		{
			name: "vuln entry page",
			url:  "https://pkg.go.dev/vuln/GO-1990-0002",
			want: &vulnPage{
				page: &VulnEntryPage{
					Entry:      testEntries[1],
					AliasLinks: aliasLinks(testEntries[1]),
				},
				template: "vuln/entry",
				title:    "GO-1990-0002",
			},
		},
		{
			name: "vuln entry page - case insensitive",
			url:  "https://pkg.go.dev/vuln/go-1990-0002",
			want: &vulnPage{
				page: &VulnEntryPage{
					Entry:      testEntries[1],
					AliasLinks: aliasLinks(testEntries[1]),
				},
				template: "vuln/entry",
				title:    "GO-1990-0002",
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			url, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			got, err := newVulnPage(ctx, url, c)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(vulnPage{}), cmpopts.IgnoreUnexported(safehtml.HTML{})); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
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
