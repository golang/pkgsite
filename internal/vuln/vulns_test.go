// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/vuln/osv"
)

func TestVulnsForPackage(t *testing.T) {
	e := osv.Entry{
		ID: "GO-1",
		Affected: []osv.Affected{{
			Package: osv.Package{Name: "bad.com"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.2.3"}}, // fixed at v1.2.3
			}},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: "bad.com",
				}, {
					Path: "bad.com/bad",
				}},
			},
		}, {
			Package: osv.Package{Name: "unfixable.com"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}}, // no fix
			}},
			DatabaseSpecific: osv.DatabaseSpecific{},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: "unfixable.com",
				}},
			},
		}},
	}
	e2 := osv.Entry{
		ID: "GO-2",
		Affected: []osv.Affected{{
			Package: osv.Package{Name: "bad.com"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.2.0"}},
			}},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: "bad.com/pkg",
				},
				},
			},
		}},
	}
	stdlib := osv.Entry{
		ID: "GO-3",
		Affected: []osv.Affected{{
			Package: osv.Package{Name: "stdlib"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.19.4"}},
			}},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: "net/http",
				}},
			},
		}},
	}

	legacyClient := newTestLegacyClient([]*osv.Entry{&e, &e2, &stdlib})
	v1Client, err := newTestClientFromTxtar("testdata/db2.txtar")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name              string
		mod, pkg, version string
		want              []Vuln
	}{
		// Vulnerabilities for a package
		{
			name: "no match - all",
			mod:  "good.com", pkg: "good.com", version: "v1.0.0",
			want: nil,
		},
		{
			name: "match - same mod/pkg",
			mod:  "bad.com", pkg: "bad.com", version: "v1.0.0",
			want: []Vuln{{ID: "GO-1"}},
		},
		{
			name: "match - different mod/pkg",
			mod:  "bad.com", pkg: "bad.com/bad", version: "v1.0.0",
			want: []Vuln{{ID: "GO-1"}},
		},
		{
			name: "no match - pkg",
			mod:  "bad.com", pkg: "bad.com/ok", version: "v1.0.0",
			want: nil, // bad.com/ok isn't affected.
		},
		{
			name: "no match - version",
			mod:  "bad.com", pkg: "bad.com", version: "v1.3.0",
			want: nil, // version 1.3.0 isn't affected
		},
		{
			name: "match - pkg with no fix",
			mod:  "unfixable.com", pkg: "unfixable.com", version: "v1.999.999", want: []Vuln{{ID: "GO-1"}},
		},
		// Vulnerabilities for a module (package == "")
		{
			name: "no match - module only",
			mod:  "good.com", pkg: "", version: "v1.0.0", want: nil,
		},
		{
			name: "match - module only",
			mod:  "bad.com", pkg: "", version: "v1.0.0", want: []Vuln{{ID: "GO-1"}, {ID: "GO-2"}},
		},
		{
			name: "no match - module but not version",
			mod:  "bad.com", pkg: "", version: "v1.3.0",
			want: nil,
		},
		{
			name: "match - module only, no fix",
			mod:  "unfixable.com", pkg: "", version: "v1.999.999", want: []Vuln{{ID: "GO-1"}},
		},
		// Vulns for stdlib
		{
			name: "match - stdlib",
			mod:  "std", pkg: "net/http", version: "go1.19.3",
			want: []Vuln{{ID: "GO-3"}},
		},
		{
			name: "no match - stdlib pseudoversion",
			mod:  "std", pkg: "net/http", version: "v0.0.0-20230104211531-bae7d772e800", want: nil,
		},
		{
			name: "no match - stdlib version past fix",
			mod:  "std", pkg: "net/http", version: "go1.20", want: nil,
		},
	}
	test := func(t *testing.T, ctx context.Context, c *Client) {
		for _, tc := range testCases {
			{
				t.Run(tc.name, func(t *testing.T) {
					got := VulnsForPackage(ctx, tc.mod, tc.version, tc.pkg, c)
					if diff := cmp.Diff(tc.want, got); diff != "" {
						t.Errorf("VulnsForPackage(mod=%q, v=%q, pkg=%q) = %+v, want %+v, diff (-want, +got):\n%s", tc.mod, tc.version, tc.pkg, got, tc.want, diff)
					}
				})
			}
		}
	}
	t.Run("legacy", func(t *testing.T) {
		test(t, context.Background(), &Client{legacy: legacyClient})
	})

	t.Run("v1", func(t *testing.T) {
		ctx := experiment.NewContext(context.Background(), internal.ExperimentVulndbV1)
		test(t, ctx, &Client{v1: v1Client})
	})
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

func TestAffectedPackages_Versions(t *testing.T) {
	for _, test := range []struct {
		name string
		in   []osv.RangeEvent
		want string
	}{
		{
			"no intro or fixed",
			nil,
			"",
		},
		{
			"no intro",
			[]osv.RangeEvent{{Fixed: "1.5"}},
			"before v1.5",
		},
		{
			"both",
			[]osv.RangeEvent{{Introduced: "1.5"}, {Fixed: "1.10"}},
			"from v1.5 before v1.10",
		},
		{
			"multiple",
			[]osv.RangeEvent{
				{Introduced: "1.5", Fixed: "1.10"},
				{Fixed: "2.3"},
			},
			"from v1.5 before v1.10, before v2.3",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := &osv.Entry{
				Affected: []osv.Affected{{
					Package: osv.Package{Name: "example.com/p"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path: "example.com/p",
						}},
					},
					Ranges: osv.Affects{{
						Type:   osv.TypeSemver,
						Events: test.in,
					}},
				}},
			}
			out := AffectedPackages(entry)
			got := out[0].Versions
			if got != test.want {
				t.Errorf("got %q, want %q\n", got, test.want)
			}
		})
	}
}

func TestAffectedPackagesPackagesSymbols(t *testing.T) {
	tests := []struct {
		name string
		in   *osv.Entry
		want []*AffectedPackage
	}{
		{
			name: "one symbol",
			in: &osv.Entry{
				ID: "GO-2022-01",
				Affected: []osv.Affected{{
					Package: osv.Package{Name: "example.com/mod"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path:    "example.com/mod/pkg",
							Symbols: []string{"F"},
						}},
					},
				}},
			},
			want: []*AffectedPackage{{
				PackagePath: "example.com/mod/pkg",
				Symbols:     []string{"F"},
			}},
		},
		{
			name: "multiple symbols",
			in: &osv.Entry{
				ID: "GO-2022-02",
				Affected: []osv.Affected{{
					Package: osv.Package{Name: "example.com/mod"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path:    "example.com/mod/pkg",
							Symbols: []string{"F", "g", "S.f", "S.F", "s.F", "s.f"},
						}},
					},
				}},
			},
			want: []*AffectedPackage{{
				PackagePath: "example.com/mod/pkg",
				Symbols:     []string{"F", "S.F"}, // unexported symbols are excluded.
			}},
		},
		{
			name: "no symbol",
			in: &osv.Entry{
				ID: "GO-2022-03",
				Affected: []osv.Affected{{
					Package: osv.Package{Name: "example.com/mod"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path: "example.com/mod/pkg",
						}},
					},
				}},
			},
			want: []*AffectedPackage{{
				PackagePath: "example.com/mod/pkg",
			}},
		},
		{
			name: "multiple pkgs and modules",
			in: &osv.Entry{
				ID: "GO-2022-04",
				Affected: []osv.Affected{{
					Package: osv.Package{Name: "example.com/mod1"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path: "example.com/mod1/pkg1",
						}, {
							Path:    "example.com/mod1/pkg2",
							Symbols: []string{"F"},
						}},
					},
				}, {
					Package: osv.Package{Name: "example.com/mod2"},
					EcosystemSpecific: osv.EcosystemSpecific{
						Imports: []osv.EcosystemSpecificImport{{
							Path:    "example.com/mod2/pkg3",
							Symbols: []string{"g", "H"},
						}},
					},
				}},
			},
			want: []*AffectedPackage{{
				PackagePath: "example.com/mod1/pkg1",
			}, {
				PackagePath: "example.com/mod1/pkg2",
				Symbols:     []string{"F"},
			}, {
				PackagePath: "example.com/mod2/pkg3",
				Symbols:     []string{"H"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AffectedPackages(tt.in)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(AffectedPackage{})); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
