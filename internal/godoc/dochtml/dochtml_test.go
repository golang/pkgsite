// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"

	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"github.com/jba/templatecheck"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var templateSource = template.TrustedSourceFromConstant("../../../content/static/dochtml")

var update = flag.Bool("update", false, "update goldens instead of checking against them")

var testRenderOptions = RenderOptions{
	FileLinkFunc:     func(string) string { return "file" },
	SourceLinkFunc:   func(ast.Node) string { return "src" },
	SinceVersionFunc: func(string) string { return "" },
}

func TestCheckTemplates(t *testing.T) {
	LoadTemplates(templateSource)
	for _, tm := range []*template.Template{bodyTemplate, outlineTemplate, sidenavTemplate} {
		if err := templatecheck.CheckSafe(tm, templateData{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRender(t *testing.T) {
	ctx := context.Background()
	LoadTemplates(templateSource)
	fset, d := mustLoadPackage("everydecl")
	parts, err := Render(ctx, fset, d, testRenderOptions)
	if err != nil {
		t.Fatal(err)
	}
	compareWithGolden(t, parts, "everydecl", *update)

	bodyDoc, err := html.Parse(strings.NewReader(parts.Body.String()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that there are no duplicate id attributes.
	t.Run("duplicate ids", func(t *testing.T) {
		testDuplicateIDs(t, bodyDoc)
	})
	t.Run("ids-and-kinds", func(t *testing.T) {
		// Check that the id and data-kind labels are right.
		testIDsAndKinds(t, bodyDoc)
	})

	wantLinks := []render.Link{
		{Href: "https://go.googlesource.com/pkgsite", Text: "pkgsite repo"},
		{Href: "https://play-with-go.dev", Text: "Play with Go"},
	}
	if diff := cmp.Diff(wantLinks, parts.Links); diff != "" {
		t.Errorf("links mismatch (-want, +got):\n%s", diff)
	}
}

func TestRenderDeprecated(t *testing.T) {
	compare := func(ctx context.Context, name string) {
		t.Helper()
		fset, d := mustLoadPackage("deprecated")
		parts, err := Render(ctx, fset, d, testRenderOptions)
		if err != nil {
			t.Fatal(err)
		}
		compareWithGolden(t, parts, name, *update)
	}

	compare(context.Background(), "deprecated-off")
	compare(experiment.NewContext(context.Background(), internal.ExperimentDeprecatedDoc), "deprecated-on")
}

func compareWithGolden(t *testing.T, parts *Parts, name string, update bool) {
	got := fmt.Sprintf("%s\n----\n%s\n----\n%s\n", parts.Body, parts.Outline, parts.MobileOutline)
	// Remove blank lines and whitespace around lines.
	var b strings.Builder
	s := bufio.NewScanner(strings.NewReader(got))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			fmt.Fprintln(&b, line)
		}
	}
	got = b.String()
	testhelper.CompareWithGolden(t, got, name+".golden", update)
}

func TestExampleRender(t *testing.T) {
	LoadTemplates(templateSource)
	ctx := context.Background()
	fset, d := mustLoadPackage("example_test")

	parts, err := Render(ctx, fset, d, testRenderOptions)
	if err != nil {
		t.Fatal(err)
	}

	htmlDoc, err := html.Parse(strings.NewReader(parts.Body.String()))
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]string)
	walk(htmlDoc, func(n *html.Node) {
		if attr(n, "class") == "Documentation-exampleDetails js-exampleContainer" {
			var b bytes.Buffer
			err := html.Render(&b, n)
			if err != nil {
				t.Fatal(err)
			}
			got[attr(n, "id")] = b.String()
		}
	})

	for _, test := range []struct {
		name   string
		htmlID string
		want   string
	}{
		{
			name:   "Executable examples (with play buttons)",
			htmlID: "example-package-StringsCompare",
			want: `<details tabindex="-1" id="example-package-StringsCompare" class="Documentation-exampleDetails js-exampleContainer">
<summary class="Documentation-exampleDetailsHeader">Example (StringsCompare) <a href="#example-package-StringsCompare">¶</a></summary>
<div class="Documentation-exampleDetailsBody">
<p>executable example
</p>

<pre class="Documentation-exampleCode">package main

import (
	&#34;fmt&#34;
	&#34;strings&#34;
)

func main() {
	// example comment
	fmt.Println(strings.Compare(&#34;a&#34;, &#34;b&#34;))
	fmt.Println(strings.Compare(&#34;a&#34;, &#34;a&#34;))
	fmt.Println(strings.Compare(&#34;b&#34;, &#34;a&#34;))

}
</pre>

<pre><span class="Documentation-exampleOutputLabel">Output:</span>

<span class="Documentation-exampleOutput">-1
0
1
</span></pre>
</div>
<div class="Documentation-exampleButtonsContainer">
        <p class="Documentation-exampleError" role="alert" aria-atomic="true"></p>
        <button class="Documentation-exampleShareButton" aria-label="Share Code">Share</button>
        <button class="Documentation-exampleFormatButton" aria-label="Format Code">Format</button>
        <button class="Documentation-exampleRunButton" aria-label="Run Code">Run</button>
      </div></details>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			diff := cmp.Diff(test.want, got[test.htmlID])
			if diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestLinkHTML(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		link string
		want string
	}{
		{
			name: "regular string and link are rendered",
			in:   `escape.go`,
			link: `https://golang.org/src/html/escape.go`,
			want: `<a class="class" href="https://golang.org/src/html/escape.go">escape.go</a>`,
		},
		{
			name: "name is escaped",
			in:   `"File & name" <'file@name.com>`,
			link: "",
			want: `&#34;File &amp; name&#34; &lt;&#39;file@name.com&gt;`,
		},
		{
			name: "link is escaped",
			in:   "file.go",
			link: `"abc@go's.com"`,
			want: `<a class="class" href="%22abc@go%27s.com%22">file.go</a>`,
		},
		{
			name: "file name and link are escaped",
			in:   `"a's.com@/`,
			link: `"x@go's.com"`,
			want: `<a class="class" href="%22x@go%27s.com%22">&#34;a&#39;s.com@/</a>`,
		},
		{
			name: "HTML injection escaped",
			in:   `<a href="gfr.con"></a>`,
			link: `"><script>bad</script>`,
			want: `<a class="class" href="%22%3e%3cscript%3ebad%3c/script%3e">&lt;a href=&#34;gfr.con&#34;&gt;&lt;/a&gt;</a>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := linkHTML(test.in, test.link, "class")
			diff := cmp.Diff(test.want, got.String())
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
		{"A", "type"},
		{"B", "type"},
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
	srcName := filepath.Base(path) + ".go"
	code, err := ioutil.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	astFile, _ := parser.ParseFile(fset, srcName, code, parser.ParseComments)
	files := []*ast.File{astFile}

	astPackage, err := doc.NewFromFiles(fset, files, path, doc.AllDecls)
	if err != nil {
		panic(err)
	}

	return fset, astPackage
}
