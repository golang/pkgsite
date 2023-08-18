// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func TestLicenseAnchors(t *testing.T) {
	for _, test := range []struct {
		in, want []string
	}{
		{[]string{"L.md"}, []string{"lic-0"}},
		// Identifiers are distinguished by the position in the sorted list.
		{[]string{"L.md", "L_md"}, []string{"lic-0", "lic-1"}},
		{[]string{"L_md", "L.md"}, []string{"lic-1", "lic-0"}},
	} {
		gotIDs := licenseAnchors(test.in)
		if len(test.want) != len(gotIDs) {
			t.Errorf("%v: mismatched lengths", test.in)
		} else {
			for i, g := range gotIDs {
				if got, want := g.String(), test.want[i]; got != want {
					t.Errorf("%v, #%d: got %q, want %q", test.in, i, got, want)
				}
			}
		}
	}
}

func TestFetchLicensesDetails(t *testing.T) {
	testModule := sample.Module(sample.ModulePath, "v1.2.3", "A/B")
	stdlibModule := sample.Module(stdlib.ModulePath, "v1.13.0", "cmd/go")
	crlfPath := "github.com/crlf/module_name"
	crlfModule := sample.Module(crlfPath, "v1.2.3", "A")

	mit := &licenses.Metadata{Types: []string{"MIT"}, FilePath: "LICENSE"}
	bsd := &licenses.Metadata{Types: []string{"BSD-3-Clause"}, FilePath: "A/B/LICENSE"}

	mitLicense := &licenses.License{
		Metadata: mit,
		Contents: []byte(testhelper.MITLicense),
	}
	mitLicenseCRLF := &licenses.License{
		Metadata: mit,
		Contents: []byte(strings.ReplaceAll(testhelper.MITLicense, "\n", "\r\n")),
	}
	bsdLicense := &licenses.License{
		Metadata: bsd,
		Contents: []byte(testhelper.BSD0License),
	}

	testModule.Licenses = []*licenses.License{bsdLicense, mitLicense}
	crlfModule.Licenses = []*licenses.License{mitLicenseCRLF}
	sort.Slice(testModule.Units, func(i, j int) bool {
		return testModule.Units[i].Path < testModule.Units[j].Path
	})

	// github.com/valid/module_name
	testModule.Units[0].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A
	testModule.Units[1].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A/B
	testModule.Units[2].Licenses = []*licenses.Metadata{mit, bsd}

	fds := fakedatasource.New()
	ctx := context.Background()
	fds.MustInsertModule(ctx, testModule)
	fds.MustInsertModule(ctx, stdlibModule)
	fds.MustInsertModule(ctx, crlfModule)
	for _, test := range []struct {
		err                                 error
		name, fullPath, modulePath, version string
		want                                []*licenses.License
	}{
		{
			name:       "module root",
			fullPath:   sample.ModulePath,
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       []*licenses.License{testModule.Licenses[1]},
		},
		{
			name:       "package without license",
			fullPath:   sample.ModulePath + "/A",
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       []*licenses.License{testModule.Licenses[1]},
		},
		{
			name:       "package with additional license",
			fullPath:   sample.ModulePath + "/A/B",
			modulePath: sample.ModulePath,
			version:    testModule.Version,
			want:       testModule.Licenses,
		},
		{
			name:       "stdlib directory",
			fullPath:   "cmd",
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
		{
			name:       "stdlib package",
			fullPath:   "cmd/go",
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
		{
			name:       "stdlib module",
			fullPath:   stdlib.ModulePath,
			modulePath: stdlib.ModulePath,
			version:    stdlibModule.Version,
			want:       stdlibModule.Licenses,
		},
		{
			name:       "module with CRLF line terminators",
			fullPath:   crlfPath,
			modulePath: crlfPath,
			version:    crlfModule.Version,
			want:       crlfModule.Licenses,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wantDetails := &LicensesDetails{Licenses: transformLicenses(
				test.modulePath, test.version, test.want)}
			got, err := fetchLicensesDetails(ctx, fds, &internal.UnitMeta{
				Path: test.fullPath,
				ModuleInfo: internal.ModuleInfo{
					ModulePath: test.modulePath,
					Version:    test.version,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantDetails, got,
				cmp.AllowUnexported(safehtml.HTML{}, safehtml.Identifier{}),
			); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			for _, l := range got.Licenses {
				if bytes.Contains(l.Contents, []byte("\r")) {
					t.Errorf("license %s contains \\r line terminators", l.Metadata.FilePath)
				}
			}
		})
	}
}
