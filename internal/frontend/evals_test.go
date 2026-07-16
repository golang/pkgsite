// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestFetchEvalsDetails(t *testing.T) {
	ctx := context.Background()

	mit := &licenses.Metadata{Types: []string{"MIT"}, FilePath: "LICENSE"}
	mitLicense := &licenses.License{
		Metadata: mit,
		Contents: []byte("MIT License"),
	}

	tests := []struct {
		name              string
		version           string
		isRedistributable bool
		licenses          []*licenses.License
		want              []eval
	}{
		{
			name:              "no license, untagged pseudo version",
			version:           "v0.0.0-20260101120000-abcdef123456",
			isRedistributable: false,
			licenses:          nil,
			want: []eval{
				{
					Type:  licenseEval,
					Score: 0,
					Value: "no license",
				},
				{
					Type:  moduleVersionEval,
					Score: 0,
					Value: "untagged",
				},
			},
		},
		{
			name:              "no license, prerelease",
			version:           "v1.2.3-alpha",
			isRedistributable: false,
			licenses:          nil,
			want: []eval{
				{
					Type:  licenseEval,
					Score: 0,
					Value: "no license",
				},
				{
					Type:  moduleVersionEval,
					Score: 1,
					Value: "tagged, unstable",
				},
			},
		},
		{
			name:              "non-redistributable license, unstable v0 tagged version",
			version:           "v0.1.0",
			isRedistributable: false,
			licenses:          []*licenses.License{mitLicense},
			want: []eval{
				{
					Type:  licenseEval,
					Score: 1,
					Value: "non-redistributable license",
				},
				{
					Type:  moduleVersionEval,
					Score: 1,
					Value: "tagged, unstable",
				},
			},
		},
		{
			name:              "redistributable license, stable v1 tagged version",
			version:           "v1.2.3",
			isRedistributable: true,
			licenses:          []*licenses.License{mitLicense},
			want: []eval{
				{
					Type:  licenseEval,
					Score: 2,
					Value: "redistributable license",
				},
				{
					Type:  moduleVersionEval,
					Score: 2,
					Value: "tagged, stable",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fds := fakedatasource.New()
			mod := sample.Module(sample.ModulePath, tt.version, sample.Suffix)
			mod.IsRedistributable = tt.isRedistributable
			mod.Licenses = tt.licenses
			for _, u := range mod.Units {
				u.IsRedistributable = tt.isRedistributable
				u.LicenseContents = tt.licenses
			}
			fds.MustInsertModule(t, mod)

			um := &internal.UnitMeta{
				Path:       sample.ModulePath + "/" + sample.Suffix,
				ModuleInfo: mod.ModuleInfo,
			}

			got, err := fetchEvalsDetails(ctx, fds, um)
			if err != nil {
				t.Fatalf("fetchEvalsDetails() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got.Evals, cmp.AllowUnexported(eval{}, evalType{})); diff != "" {
				t.Errorf("fetchEvalsDetails() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAgeString(t *testing.T) {
	testCases := []struct {
		d    time.Duration
		want string
	}{
		{0, "less than a day"},
		{12 * time.Hour, "less than a day"},
		{23*time.Hour + 59*time.Minute, "less than a day"},
		{day, "1 day"},
		{2 * day, "2 days"},
		{23 * day, "23 days"},
		{30 * day, "30 days"},
		{31 * day, "1 month"},
		{31*day + 1*day, "1 month, 1 day"},
		{31*day + 5*day, "1 month, 5 days"},
		{2 * month, "2 months"},
		{2*month + 1*day, "2 months, 1 day"},
		{2*month + 3*day, "2 months, 3 days"},
		{11*month + 23*day, "11 months, 23 days"},
		{year, "1 year"},
		{year + 1*day, "1 year"}, // 1 day is 0 months remainder
		{year + 1*month, "1 year, 1 month"},
		{year + 2*month + 5*day, "1 year, 2 months"},
		{2 * year, "2 years"},
		{2*year + 5*month, "2 years, 5 months"},
		{2*year + 1*month + 30*day, "2 years, 1 month"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got := ageString(tc.d)
			if got != tc.want {
				t.Errorf("ageString(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestSummarizeDocumentation(t *testing.T) {
	testCases := []struct {
		name  string
		files map[string]string
		want  docSummary
	}{
		{
			name:  "nil or empty",
			files: nil,
			want:  docSummary{},
		},
		{
			name: "only test files ignored",
			files: map[string]string{
				"foo_test.go": `package foo
// TestSomething tests something.
func TestSomething(t *testing.T) {}`,
			},
			want: docSummary{},
		},
		{
			name: "main package ignored",
			files: map[string]string{
				"main.go": `package main
// Package main is executable.
func main() {}`,
			},
			want: docSummary{},
		},
		{
			name: "package has doc",
			files: map[string]string{
				"doc.go": `// Package mypkg provides utility routines.
package mypkg`,
			},
			want: docSummary{
				packageHasDoc: true,
			},
		},
		{
			name: "package doesn't have doc",
			files: map[string]string{
				"doc.go": `package mypkg`,
			},
			want: docSummary{
				packageHasDoc: false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var pkg *godoc.Package
			if tc.files != nil {
				pkg = parseTestPackage(t, tc.files)
			}
			got := summarizeDocumentation(pkg)
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(docSummary{})); diff != "" {
				t.Errorf("summarizeDocumentation() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// parseTestPackage creates a godoc.Package with the given files. It removes the
// bodies of functions, encodes and then decodes the package, to simulate what the
// frontend actually observes. It returns the decoded package.
func parseTestPackage(t *testing.T, files map[string]string) *godoc.Package {
	t.Helper()
	fset := token.NewFileSet()
	docPkg := godoc.NewPackage(fset, nil)
	for name, src := range files {
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q): %v", name, err)
		}
		docPkg.AddFile(f, true)
	}
	bytes, err := docPkg.Encode(context.Background())
	if err != nil {
		t.Fatalf("docPkg.Encode: %v", err)
	}
	decodedPkg, err := godoc.DecodePackage(bytes)
	if err != nil {
		t.Fatalf("godoc.DecodePackage: %v", err)
	}
	return decodedPkg
}
