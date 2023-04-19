// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/tools/txtar"
)

const (
	dbTxtar = "testdata/db.txtar"
)

var (
	jan1999  = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	jan2000  = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	jan2002  = time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC)
	jan2003  = time.Date(2003, 1, 1, 0, 0, 0, 0, time.UTC)
	testOSV1 = osv.Entry{
		ID:        "GO-1999-0001",
		Published: jan1999,
		Modified:  jan2000,
		Aliases:   []string{"CVE-1999-1111"},
		Details:   "Some details",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path:      "stdlib",
					Ecosystem: "Go",
				},
				Ranges: []osv.Range{
					osv.Range{
						Type: "SEMVER",
						Events: []osv.RangeEvent{
							{Introduced: "0"}, {Fixed: "1.1.0"},
							{Introduced: "1.2.0"},
							{Fixed: "1.2.2"},
						}}},
				EcosystemSpecific: osv.EcosystemSpecific{
					Packages: []osv.Package{{Path: "package", Symbols: []string{"Symbol"}}}}},
		},
		References: []osv.Reference{
			{Type: "FIX", URL: "https://example.com/cl/123"},
		},
		DatabaseSpecific: &osv.DatabaseSpecific{
			URL: "https://pkg.go.dev/vuln/GO-1999-0001"},
	}
	testOSV2 = osv.Entry{
		ID:        "GO-2000-0002",
		Published: jan2000,
		Modified:  jan2002,
		Aliases:   []string{"CVE-1999-2222"},
		Details:   "Some details",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path:      "example.com/module",
					Ecosystem: "Go",
				},
				Ranges: []osv.Range{
					osv.Range{
						Type: "SEMVER", Events: []osv.RangeEvent{{Introduced: "0"},
							{Fixed: "1.2.0"},
						}}},
				EcosystemSpecific: osv.EcosystemSpecific{
					Packages: []osv.Package{{Path: "example.com/module/package",
						Symbols: []string{"Symbol"},
					}}}}},
		References: []osv.Reference{
			{Type: "FIX", URL: "https://example.com/cl/543"},
		},
		DatabaseSpecific: &osv.DatabaseSpecific{URL: "https://pkg.go.dev/vuln/GO-2000-0002"},
	}
	testOSV3 = osv.Entry{
		ID:        "GO-2000-0003",
		Published: jan2000,
		Modified:  jan2003,
		Aliases:   []string{"CVE-1999-3333", "GHSA-xxxx-yyyy-zzzz"},
		Details:   "Some details",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path:      "example.com/module",
					Ecosystem: "Go",
				},
				Ranges: []osv.Range{
					{
						Type: "SEMVER",
						Events: []osv.RangeEvent{
							{Introduced: "0"}, {Fixed: "1.1.0"},
						}}},
				EcosystemSpecific: osv.EcosystemSpecific{Packages: []osv.Package{
					{
						Path:    "example.com/module/package",
						Symbols: []string{"Symbol"},
					},
					{
						Path: "example.com/module/package2",
					},
				}}}},
		References: []osv.Reference{
			{Type: "FIX", URL: "https://example.com/cl/000"},
		},
		DatabaseSpecific: &osv.DatabaseSpecific{
			URL: "https://pkg.go.dev/vuln/GO-2000-0003",
		}}
)

func TestByPackage(t *testing.T) {
	c, err := newTestClientFromTxtar(dbTxtar)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		req  *PackageRequest
		want []*osv.Entry
	}{
		{
			name: "match on package",
			req: &PackageRequest{
				Module:  "example.com/module",
				Package: "example.com/module/package2",
			},
			want: []*osv.Entry{&testOSV3},
		},
		{
			// package affects OSV2 and OSV3, but version
			// only applies to OSV2
			name: "match on package version",
			req: &PackageRequest{
				Module:  "example.com/module",
				Package: "example.com/module/package",
				Version: "1.1.0",
			},
			want: []*osv.Entry{&testOSV2},
		},
		{
			// when the package is not specified, only the
			// module is used.
			name: "match on module",
			req: &PackageRequest{
				Module:  "example.com/module",
				Package: "",
				Version: "1.0.0",
			},
			want: []*osv.Entry{&testOSV2, &testOSV3},
		},
		{
			name: "stdlib",
			req: &PackageRequest{
				Module:  "stdlib",
				Package: "package",
				Version: "1.0.0",
			},
			want: []*osv.Entry{&testOSV1},
		},
		{
			// when no version is specified, all entries for the module
			// should show up
			name: "no version",
			req: &PackageRequest{
				Module: "stdlib",
			},
			want: []*osv.Entry{&testOSV1},
		},
		{
			name: "unaffected version",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "3.0.0",
			},
			want: nil,
		},
		{
			name: "v prefix ok - in range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "v1.0.0",
			},
			want: []*osv.Entry{&testOSV1},
		},
		{
			name: "v prefix ok - out of range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "v3.0.0",
			},
			want: nil,
		},
		{
			name: "go prefix ok - in range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "go1.0.0",
			},
			want: []*osv.Entry{&testOSV1},
		},
		{
			name: "go prefix ok - out of range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "go3.0.0",
			},
			want: nil,
		},
		{
			name: "go prefix, no patch version - in range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "go1.2",
			},
			want: []*osv.Entry{&testOSV1},
		},
		{
			name: "go prefix, no patch version - out of range",
			req: &PackageRequest{
				Module:  "stdlib",
				Version: "go1.3",
			},
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := c.ByPackage(ctx, test.req)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("ByPackage(%s) = %s, want %s", test.req, ids(got), ids(test.want))
			}
		})
	}
}

func ids(entries []*osv.Entry) string {
	var ids []string
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return "[" + strings.Join(ids, ",") + "]"
}

func TestByAlias(t *testing.T) {
	c, err := newTestClientFromTxtar(dbTxtar)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		alias   string
		want    string
		wantErr bool
	}{
		{
			name:  "CVE",
			alias: "CVE-1999-1111",
			want:  testOSV1.ID,
		},
		{
			name:  "GHSA",
			alias: "GHSA-xxxx-yyyy-zzzz",
			want:  testOSV3.ID,
		},
		{
			name:    "Not found",
			alias:   "CVE-0000-0000",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := c.ByAlias(ctx, test.alias)
			if !test.wantErr {
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, test.want) {
					t.Errorf("ByAlias(%s) = %v, want %v", test.alias, got, test.want)
				}
			} else if err == nil {
				t.Errorf("ByAlias(%s) = %v, want error", test.alias, got)
			}
		})
	}
}

func TestByID(t *testing.T) {
	c, err := newTestClientFromTxtar(dbTxtar)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id   string
		want *osv.Entry
	}{
		{
			id:   testOSV1.ID,
			want: &testOSV1,
		},
		{
			id:   testOSV2.ID,
			want: &testOSV2,
		},
		{
			id:   "invalid",
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			ctx := context.Background()
			got, err := c.ByID(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("ByID(%s) = %v, want %v", test.id, got, test.want)
			}
		})
	}
}

func TestEntries(t *testing.T) {
	ctx := context.Background()
	c, err := newTestClientFromTxtar(dbTxtar)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		n    int
		want []*osv.Entry
	}{
		{
			n:    -1,
			want: []*osv.Entry{&testOSV3, &testOSV2, &testOSV1},
		},
		{
			n:    0,
			want: nil,
		},
		{
			n:    2,
			want: []*osv.Entry{&testOSV3, &testOSV2},
		},
		{
			n:    3,
			want: []*osv.Entry{&testOSV3, &testOSV2, &testOSV1},
		},
		{
			n:    4,
			want: []*osv.Entry{&testOSV3, &testOSV2, &testOSV1},
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("n=%d", tc.n), func(t *testing.T) {
			got, err := c.Entries(ctx, tc.n)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Entries = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestByPackagePrefix(t *testing.T) {
	stdlibNet := &osv.Entry{
		ID: "1-STDLIB-NET",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path: "golang.org/x/example",
				},
			},
			{
				Module: osv.Module{
					Path: "stdlib",
				},
				EcosystemSpecific: osv.EcosystemSpecific{
					Packages: []osv.Package{
						{
							Path: "net/http/httputil",
						},
					},
				},
			},
		},
	}
	stdlibCrypto := &osv.Entry{
		ID: "2-STDLIB-CRYPTO",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path: "golang.org/x/example",
				},
			},
			{
				Module: osv.Module{
					Path: "stdlib",
				},
				EcosystemSpecific: osv.EcosystemSpecific{
					Packages: []osv.Package{
						{
							Path: "crypto/tls",
						},
					},
				},
			},
		},
	}
	thirdParty := &osv.Entry{
		ID: "3-EXAMPLE-COM",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path: "golang.org/x/example",
				},
			},
			{

				Module: osv.Module{
					Path: "example.com/org/module",
				},
				EcosystemSpecific: osv.EcosystemSpecific{
					Packages: []osv.Package{
						{
							Path: "example.com/org/module/somepkg",
						},
						{
							Path: "example.com/org/module/package/inner",
						},
					},
				},
			},
		},
	}
	// Entry containing two modules with a common prefix.
	commonPrefix := &osv.Entry{
		ID: "4-COMMON-PREFIX",
		Affected: []osv.Affected{
			{
				Module: osv.Module{
					Path: "example.com/module",
				},
			},
			{
				Module: osv.Module{
					Path: "example.com/module/inner",
				},
			},
		},
	}
	vc, err := NewInMemoryClient([]*osv.Entry{stdlibCrypto, stdlibNet, thirdParty, commonPrefix})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		query string
		want  []*osv.Entry
	}{
		{
			name:  "no match",
			query: "net/htt",
			want:  nil,
		},
		{
			name:  "stdlib module exact match",
			query: "stdlib",
			want:  []*osv.Entry{stdlibCrypto, stdlibNet},
		},
		{
			name:  "stdlib package exact match",
			query: "net/http/httputil",
			want:  []*osv.Entry{stdlibNet},
		},
		{
			name:  "stdlib package prefix match",
			query: "net/http",
			want:  []*osv.Entry{stdlibNet},
		},
		{
			name:  "3p module exact match",
			query: "example.com/org/module",
			want:  []*osv.Entry{thirdParty},
		},
		{
			name:  "3p module prefix match",
			query: "example.com/org",
			want:  []*osv.Entry{thirdParty},
		},
		{
			name:  "3p package exact match",
			query: "example.com/org/module/package/inner",
			want:  []*osv.Entry{thirdParty},
		},
		{
			name:  "3p package prefix match",
			query: "example.com/org/module/package",
			want:  []*osv.Entry{thirdParty},
		},
		{
			name:  "prefix with trailing slash",
			query: "example.com/org/module/package/",
			want:  []*osv.Entry{thirdParty},
		},
		{
			name:  "descending order by ID",
			query: "golang.org/x",
			want:  []*osv.Entry{thirdParty, stdlibCrypto, stdlibNet},
		},
		{
			name: "entries not duplicated",
			// Query is both an exact match and a prefix for another
			// module, but entry should only show up once.
			query: "example.com/module",
			want:  []*osv.Entry{commonPrefix},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := vc.ByPackagePrefix(context.Background(), tc.query)
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(got, tc.want) {
				t.Errorf("ByPackagePrefix(%s) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// newTestClientFromTxtar creates an in-memory client for use in tests.
// It reads test data from a txtar file which must follow the
// v1 database schema.
func newTestClientFromTxtar(txtarFile string) (*Client, error) {
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

	return &Client{&inMemorySource{data: data}}, nil
}

func removeWhitespace(data []byte) ([]byte, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
