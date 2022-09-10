// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulns

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/vuln/osv"
)

func TestVulnsForPackage(t *testing.T) {
	ctx := context.Background()
	e := osv.Entry{
		Details: "bad",
		Affected: []osv.Affected{{
			Package: osv.Package{Name: "bad.com"},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.2.3"}},
			}},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: "bad.com",
				}},
			},
		}},
	}

	get := func(_ context.Context, modulePath string) ([]*osv.Entry, error) {
		switch modulePath {
		case "good.com":
			return nil, nil
		case "bad.com":
			return []*osv.Entry{&e}, nil
		default:
			return nil, fmt.Errorf("unknown module %q", modulePath)
		}
	}

	got := VulnsForPackage(ctx, "good.com", "v1.0.0", "good.com", get)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	got = VulnsForPackage(ctx, "bad.com", "v1.0.0", "bad.com", get)
	want := []Vuln{{
		Details: "bad",
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}

	got = VulnsForPackage(ctx, "bad.com", "v1.3.0", "bad.com", get)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
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
