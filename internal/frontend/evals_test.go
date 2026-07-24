// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/tools/txtar"
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
		name string
		// The files of the package. If nil, read the txtar from from testdata whose name
		// is the test case name, replacing spaces with underscores.
		files map[string]string
		want  docSummary
	}{
		{
			name:  "empty",
			files: map[string]string{},
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
			name: "package does not have doc",
			files: map[string]string{
				"doc.go": `package mypkg`,
			},
			want: docSummary{
				packageHasDoc: false,
			},
		},
		{
			name:  "mixed doc and undoc",
			files: nil,
			want: docSummary{
				packageHasDoc:      true,
				numExportedSymbols: 8,
				numHaveDoc:         3,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := tc.files
			if files == nil {
				txtarPath := filepath.Join("testdata", strings.ReplaceAll(tc.name, " ", "_")+".txtar")
				data, err := os.ReadFile(txtarPath)
				if err != nil {
					t.Fatalf("os.ReadFile(%q): %v", txtarPath, err)
				}
				archive := txtar.Parse(data)
				files = make(map[string]string)
				for _, f := range archive.Files {
					files[f.Name] = string(f.Data)
				}
			}

			pkg := parseTestPackage(t, files)
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
	if len(files) == 0 {
		return nil
	}
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

func TestCollectSymbols(t *testing.T) {
	src := `package mypkg

// ExportedFunc has doc.
func ExportedFunc() {}

func unexportedFunc() {}

// ExportedType has doc.
type ExportedType struct{}

type unexportedType struct{}

// ExportedMethod has doc.
func (e ExportedType) ExportedMethod() {}

func (e ExportedType) UndocumentedMethod() {}

// MethodOnUnexportedType should be ignored.
func (u unexportedType) MethodOnUnexportedType() {}

// ExportedConst has doc.
const ExportedConst = 1

const UndocumentedConst = 2

// SingleGroupedType applies doc to single spec.
type SingleGroupedType int

var (
	// ExportedVar has doc.
	ExportedVar = "a"
	UndocumentedVar = "b" // UndocumentedVar has line doc
	NoDocVar = "c"
	unexportedVar = "d"
)
`

	pkg := parseTestPackage(t, map[string]string{"mypkg.go": src})
	var files []*ast.File
	for _, f := range pkg.Files {
		if f != nil && f.AST != nil {
			files = append(files, f.AST)
		}
	}

	got := make(map[string]bool)
	collectSymbols(files, func(name string, has bool) {
		got[name] = has
	})

	want := map[string]bool{
		"ExportedFunc":                      true,
		"ExportedType":                      true,
		"(ExportedType).ExportedMethod":     true,
		"(ExportedType).UndocumentedMethod": false,
		"ExportedConst":                     true,
		"UndocumentedConst":                 false,
		"SingleGroupedType":                 true,
		"ExportedVar":                       true,
		"UndocumentedVar":                   true,
		"NoDocVar":                          false,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("collectSymbols() mismatch (-want +got):\n%s", diff)
	}
}
func TestSigString(t *testing.T) {
	testCases := []struct {
		src  string
		want string
	}{
		{"package p; func Foo(int, int) bool", "(int, int) bool"},
		{"package p; func Foo(x, y int) (z bool)", "(int, int) bool"},
		{"package p; func Foo()", "()"},
		{"package p; func Foo(a string, b, c int, d ...byte) (error, int)", "(string, int, int, ...byte) (error, int)"},
		{"package p; func Foo() (x, y int)", "() (int, int)"},
		{"package p; func Foo() (bool)", "() bool"},
	}
	for _, tc := range testCases {
		t.Run(tc.src, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile(%q): %v", tc.src, err)
			}
			fd := f.Decls[0].(*ast.FuncDecl)
			got := sigString(fd.Type)
			if got != tc.want {
				t.Errorf("sigString(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestRecvTypeName(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "value receiver",
			src:  "package p; func (t MyType) M() {}",
			want: "MyType",
		},
		{
			name: "pointer receiver",
			src:  "package p; func (t *MyType) M() {}",
			want: "MyType",
		},
		{
			name: "generic value receiver with single type parameter",
			src:  "package p; func (t MyType[P]) M() {}",
			want: "MyType",
		},
		{
			name: "generic pointer receiver with single type parameter",
			src:  "package p; func (t *MyType[P]) M() {}",
			want: "MyType",
		},
		{
			name: "generic value receiver with multiple type parameters",
			src:  "package p; func (t MyType[P1, P2]) M() {}",
			want: "MyType",
		},
		{
			name: "generic pointer receiver with multiple type parameters",
			src:  "package p; func (t *MyType[P1, P2]) M() {}",
			want: "MyType",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile: %v", err)
			}
			decl := f.Decls[0].(*ast.FuncDecl)
			got := recvTypeName(decl.Recv)
			if got != tc.want {
				t.Errorf("recvTypeName(%s) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
