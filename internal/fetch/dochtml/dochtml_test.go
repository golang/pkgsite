// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
)

func TestRender(t *testing.T) {
	fset, d := mustLoadPackage("everydecl")

	rawDoc, err := Render(fset, d, RenderOptions{
		FileLinkFunc:   func(string) string { return "file" },
		SourceLinkFunc: func(ast.Node) string { return "src" },
	})
	if err != nil {
		t.Fatal(err)
	}
	htmlDoc, err := html.Parse(strings.NewReader(rawDoc))
	if err != nil {
		t.Fatal(err)
	}
	// Check that there are no duplicate id attributes.
	t.Run("duplicate ids", func(t *testing.T) {
		testDuplicateIDs(t, htmlDoc)
	})
	t.Run("ids-and-kinds", func(t *testing.T) {
		// Check that the id and data-kind labels are right.
		testIDsAndKinds(t, htmlDoc)
	})
}

func TestFileLinkHTML(t *testing.T) {
	for _, test := range []struct {
		name string
		file string
		link string
		want template.HTML
	}{
		{
			name: "file name is escaped",
			file: `"File & name" <'file@name.com>`,
			link: "",
			want: `&#34;File &amp; name&#34; &lt;&#39;file@name.com&gt;`,
		},
		{
			name: "link is escaped",
			file: "file.go",
			link: `"abc@go's.com"`,
			want: `<a class="Documentation-file" href="&#34;abc@go&#39;s.com&#34;">file.go</a>`,
		},
		{
			name: "file name and link are escaped",
			file: `"a's.com@/`,
			link: `"x@go's.com"`,
			want: `<a class="Documentation-file" href="&#34;x@go&#39;s.com&#34;">&#34;a&#39;s.com@/</a>`,
		},
		{
			name: "HTML injection escaped",
			file: `<a href="gfr.con"></a>`,
			link: `a.com`,
			want: `<a class="Documentation-file" href="a.com">&lt;a href=&#34;gfr.con&#34;&gt;&lt;/a&gt;</a>`,
		},
		{
			name: "regular file name and link are rendered",
			file: `escape.go`,
			link: `https://golang.org/src/html/escape.go`,
			want: `<a class="Documentation-file" href="https://golang.org/src/html/escape.go">escape.go</a>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := fileLinkHTML(test.file, test.link)
			diff := cmp.Diff(test.want, got)
			if diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestVersionedPkgPath(t *testing.T) {
	for _, test := range []struct {
		name    string
		pkgPath string
		modInfo *ModuleInfo
		want    string
	}{
		{
			name:    "builtin package is not versioned",
			pkgPath: "builtin",
			modInfo: &ModuleInfo{
				ModulePath:      "std",
				ResolvedVersion: "v1.14.4",
				ModulePackages:  map[string]bool{"std/builtin": true, "std/net/http": true},
			},
			want: "builtin",
		},
		{
			name:    "std packages are not versioned",
			pkgPath: "net/http",
			modInfo: &ModuleInfo{
				ModulePath:      "std",
				ResolvedVersion: "v1.14.4",
				ModulePackages:  map[string]bool{"std/builtin": true, "std/net/http": true},
			},
			want: "net/http",
		},
		{
			name:    "imports from other modules are not versioned",
			pkgPath: "golang.org/x/pkgsite",
			modInfo: &ModuleInfo{
				ModulePath:      "cloud.google.com/go",
				ResolvedVersion: "v0.60.0",
				ModulePackages:  map[string]bool{"cloud.google.com/go/civil": true},
			},
			want: "golang.org/x/pkgsite",
		},
		{
			name:    "imports from other modules with shared prefixes are not versioned",
			pkgPath: "golang.org/x/pkgsite",
			modInfo: &ModuleInfo{
				ModulePath:      "golang.org/x/time",
				ResolvedVersion: "v1.2.3",
				ModulePackages:  map[string]bool{"golang.org/x/time/rate": true},
			},
			want: "golang.org/x/pkgsite",
		},
		{
			name:    "imports from same module are versioned",
			pkgPath: "golang.org/x/pkgsite/internal/log",
			modInfo: &ModuleInfo{
				ModulePath:      "golang.org/x/pkgsite",
				ResolvedVersion: "v1.1.2",
				ModulePackages:  map[string]bool{"golang.org/x/pkgsite/internal/log": true},
			},
			want: "golang.org/x/pkgsite@v1.1.2/internal/log",
		},
		{
			name:    "imports from same module with pseudo version are versioned",
			pkgPath: "golang.org/x/pkgsite/internal/log",
			modInfo: &ModuleInfo{
				ModulePath:      "golang.org/x/pkgsite",
				ResolvedVersion: "v0.0.0-20200709011933-a59b4ce778c4",
				ModulePackages:  map[string]bool{"golang.org/x/pkgsite/internal/log": true},
			},
			want: "golang.org/x/pkgsite@v0.0.0-20200709011933-a59b4ce778c4/internal/log",
		},
		{
			name:    "imports from same v2 module are versioned",
			pkgPath: "k8s.io/klog/v2/klogr",
			modInfo: &ModuleInfo{
				ModulePath:      "k8s.io/klog/v2",
				ResolvedVersion: "v2.3.0",
				ModulePackages:  map[string]bool{"k8s.io/klog/v2": true, "k8s.io/klog/v2/klogr": true},
			},
			want: "k8s.io/klog/v2@v2.3.0/klogr",
		},
		{
			name:    "imports from older major module version are not versioned",
			pkgPath: "rsc.io/quote",
			modInfo: &ModuleInfo{
				ModulePath:      "rsc.io/quote/v3",
				ResolvedVersion: "v3.1.0",
				ModulePackages:  map[string]bool{"rsc.io/quote/v3": true},
			},
			want: "rsc.io/quote",
		},
		{
			name:    "imports from newer major module version are not versioned",
			pkgPath: "rsc.io/quote/v3",
			modInfo: &ModuleInfo{
				ModulePath:      "rsc.io/quote",
				ResolvedVersion: "v1.5.3",
				ModulePackages:  map[string]bool{"rsc.io/quote": true},
			},
			want: "rsc.io/quote/v3",
		},
		{
			name:    "imports from nested module are not versioned",
			pkgPath: "A/B/C/D",
			modInfo: &ModuleInfo{
				ModulePath:      "A",
				ResolvedVersion: "v1.0.0",
				ModulePackages:  map[string]bool{"A/B": true, "A/B/C": true},
			},
			want: "A/B/C/D",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := versionedPkgPath(test.pkgPath, test.modInfo)
			if got != test.want {
				t.Errorf("versionedPkgPath(%q) = %q, want %q", test.pkgPath, got, test.want)
			}
		})
	}
}

func testDuplicateIDs(t *testing.T, htmlDoc *html.Node) {
	idCounts := map[string]int{}
	walk(htmlDoc, func(n *html.Node) {
		id := attr(n, "id")
		if id != "" {
			idCounts[id]++
		}
	})
	var dups []string
	for id, n := range idCounts {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	if len(dups) > 0 {
		t.Errorf("duplicate ids: %v", dups)
	}
}

func testIDsAndKinds(t *testing.T, htmlDoc *html.Node) {
	type attrs struct {
		ID, Kind string // export fields for cmp
	}

	// want is a complete list of id, kind pairs we expect to see the HTML.
	want := []attrs{
		{"C", "constant"},
		{"CT", "constant"},
		{"F", "function"},
		{"TF", "function"},
		{"T.M", "method"},
		{"V", "variable"},
		{"VT", "variable"},
		{"T", "type"},
		{"S1", "type"},
		{"S1.F", "field"},
		{"S2", "type"},
		{"S2.S1", "field"},
		{"S2.G", "field"},
		{"I1", "type"},
		{"I1.M1", "method"},
		{"I2", "type"},
		{"I2.M2", "method"},
	}

	var got []attrs
	walk(htmlDoc, func(n *html.Node) {
		if kind := attr(n, "data-kind"); kind != "" {
			got = append(got, attrs{attr(n, "id"), kind})
		}
	})

	diff := cmp.Diff(want, got, cmpopts.SortSlices(func(a1, a2 attrs) bool {
		return a1.ID < a2.ID
	}))
	if diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// Copied from internal/render/render_test.go, with the slight modification of returning the fset.
func mustLoadPackage(path string) (*token.FileSet, *doc.Package) {
	// simpleImporter is used by ast.NewPackage.
	simpleImporter := func(imports map[string]*ast.Object, pkgPath string) (*ast.Object, error) {
		pkg := imports[pkgPath]
		if pkg == nil {
			pkgName := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
			pkg = ast.NewObj(ast.Pkg, pkgName)
			pkg.Data = ast.NewScope(nil) // required for or dot-imports
			imports[pkgPath] = pkg
		}
		return pkg, nil
	}

	srcName := filepath.Base(path) + ".go"
	code, err := ioutil.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	pkgFiles := make(map[string]*ast.File)
	astFile, _ := parser.ParseFile(fset, srcName, code, parser.ParseComments)
	pkgFiles[srcName] = astFile
	astPkg, _ := ast.NewPackage(fset, pkgFiles, simpleImporter, nil)
	return fset, doc.New(astPkg, path, 0)
}
