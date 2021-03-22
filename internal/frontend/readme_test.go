// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestReadme(t *testing.T) {
	ctx := experiment.NewContext(context.Background())
	unit := sample.UnitEmpty(sample.PackagePath, sample.ModulePath, sample.VersionString)
	for _, test := range []struct {
		name        string
		unit        *internal.Unit
		readme      *internal.Readme
		wantHTML    string
		wantOutline []*Heading
	}{
		{
			name: "Top level heading of h4 becomes h3, and following header levels become hN-1",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "#### Four\n\n##### Five",
			},
			wantHTML: "<h3 class=\"h4\" id=\"readme-four\">Four</h3>\n" +
				"<h4 class=\"h5\" id=\"readme-five\">Five</h4>",
			wantOutline: []*Heading{
				{Level: 4, Text: "Four", ID: "readme-four"},
				{Level: 5, Text: "Five", ID: "readme-five"},
			},
		},
		{
			name: "The top 2 level headings are kept within outline",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "#### Four\n\n##### Five\n\n###### Six",
			},
			wantHTML: "<h3 class=\"h4\" id=\"readme-four\">Four</h3>\n" +
				"<h4 class=\"h5\" id=\"readme-five\">Five</h4>\n" +
				"<h5 class=\"h6\" id=\"readme-six\">Six</h5>",
			wantOutline: []*Heading{
				{Level: 4, Text: "Four", ID: "readme-four"},
				{Level: 5, Text: "Five", ID: "readme-five"},
			},
		},
		{
			name: "Heading levels out of order",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "## Two\n\n# One\n\n### Three",
			},
			wantHTML: "<h3 class=\"h2\" id=\"readme-two\">Two</h3>\n" +
				"<h2 class=\"h1\" id=\"readme-one\">One</h2>\n" +
				"<h4 class=\"h3\" id=\"readme-three\">Three</h4>",
			wantOutline: []*Heading{
				{Level: 2, Text: "Two", ID: "readme-two"},
				{Level: 1, Text: "One", ID: "readme-one"},
			},
		},
		{
			name: "Heading levels not consecutive",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "# One\n\n#### Four\n\n#### Four",
			},
			wantHTML: "<h3 class=\"h1\" id=\"readme-one\">One</h3>\n" +
				"<h6 class=\"h4\" id=\"readme-four\">Four</h6>\n" +
				"<h6 class=\"h4\" id=\"readme-four-1\">Four</h6>",
			wantOutline: []*Heading{
				{Level: 1, Text: "One", ID: "readme-one"},
				{Level: 4, Text: "Four", ID: "readme-four"},
				{Level: 4, Text: "Four", ID: "readme-four-1"},
			},
		},
		{
			name: "Heading levels in reverse",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "### Three\n\n## Two\n\n# One",
			},
			wantHTML: "<h3 class=\"h3\" id=\"readme-three\">Three</h3>\n" +
				"<h2 class=\"h2\" id=\"readme-two\">Two</h2>\n" +
				"<h1 class=\"h1\" id=\"readme-one\">One</h1>",
			wantOutline: []*Heading{
				{Level: 2, Text: "Two", ID: "readme-two"},
				{Level: 1, Text: "One", ID: "readme-one"},
			},
		},
		{
			name: "Github markdown emoji markup is properly rendered",
			unit: unit,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "# :zap: Zap \n\n :joy:",
			},
			wantHTML: "<h3 class=\"h1\" id=\"readme-zap-zap\">⚡ Zap</h3>\n<p>😂</p>",
			wantOutline: []*Heading{
				{Level: 1, Text: " Zap", ID: "readme-zap-zap"},
			},
		},
		{
			name: "valid markdown readme",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			wantHTML: "<p>This package collects pithy sayings.</p>\n" +
				"<p>It&#39;s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>`,
			wantOutline: nil,
		},
		{
			name: "valid markdown readme with alternative case and extension",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README.MARKDOWN",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			wantHTML: "<p>This package collects pithy sayings.</p>\n" +
				"<p>It&#39;s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>`,
			wantOutline: nil,
		},
		{
			name: "valid markdown readme with CRLF",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "This package collects pithy sayings.\r\n\r\n" +
					"- It's part of a demonstration of\r\n" +
					"- [package versioning in Go](https://research.swtch.com/vgo1).",
			},
			wantHTML: "<p>This package collects pithy sayings.</p>\n" +
				"<ul>\n" +
				"<li>It&#39;s part of a demonstration of</li>\n" +
				`<li><a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</li>` + "\n" +
				"</ul>",
			wantOutline: nil,
		},
		{
			name: "not markdown readme",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README.rst",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			wantHTML: "<pre class=\"readme\">This package collects pithy sayings.\n\n" +
				"It&#39;s part of a demonstration of\n[package versioning in Go](https://research.swtch.com/vgo1).</pre>",
			wantOutline: nil,
		},
		{
			name:        "empty readme",
			unit:        &internal.Unit{},
			wantHTML:    "",
			wantOutline: nil,
		},
		{
			name: "sanitized readme",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README",
				Contents: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`,
			},
			wantHTML:    `<pre class="readme">&lt;a onblur=&#34;alert(secret)&#34; href=&#34;http://www.google.com&#34;&gt;Google&lt;/a&gt;</pre>`,
			wantOutline: nil,
		},
		{
			name: "relative image markdown is made absolute for GitHub",
			unit: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						SourceInfo: source.NewGitHubInfo("http://github.com/golang/go", "", "master"),
					},
				},
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Go logo](doc/logo.png)",
			},
			wantHTML:    `<p><img src="http://github.com/golang/go/raw/master/doc/logo.png" alt="Go logo"/></p>`,
			wantOutline: nil,
		},
		{
			name: "relative image markdown is made absolute for GitHub, .git removed from repo URL",
			unit: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						SourceInfo: source.NewGitHubInfo("https://github.com/robpike/ivy.git", "", "v0.1.0"),
					},
				},
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![ivy](ivy.jpg)",
			},
			wantHTML:    `<p><img src="https://github.com/robpike/ivy/raw/v0.1.0/ivy.jpg" alt="ivy"/></p>`,
			wantOutline: nil,
		},
		{
			name: "relative image markdown is left alone for unknown origins",
			unit: &internal.Unit{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Go logo](doc/logo.png)",
			},
			wantHTML:    `<p><img src="doc/logo.png" alt="Go logo"/></p>`,
			wantOutline: nil,
		},
		{
			name: "module versions are referenced in relative images",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Hugo logo](doc/logo.png)",
			},
			wantHTML:    `<p><img src="https://github.com/valid/module_name/raw/v1.0.0/doc/logo.png" alt="Hugo logo"/></p>`,
			wantOutline: nil,
		},
		{
			name: "image URLs relative to README directory",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				Contents: "![alt](img/thing.png)",
			},
			wantHTML:    `<p><img src="https://github.com/valid/module_name/raw/v1.0.0/dir/sub/img/thing.png" alt="alt"/></p>`,
			wantOutline: nil,
		},
		{
			name: "non-image links relative to README directory",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				Contents: "[something](doc/thing.md)",
			},
			wantHTML:    `<p><a href="https://github.com/valid/module_name/blob/v1.0.0/dir/sub/doc/thing.md" rel="nofollow">something</a></p>`,
			wantOutline: nil,
		},
		{
			name: "image link in embedded HTML",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<img src=\"resources/logoSmall.png\" />\n\n# Heading\n",
			},
			wantHTML: `<p><img src="https://github.com/valid/module_name/raw/v1.0.0/resources/logoSmall.png"/></p>` + "\n" +
				`<h3 class="h1" id="readme-heading">Heading</h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading", ID: "readme-heading"},
			},
		},
		{
			name: "image link in embedded HTML with surrounding p tag",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<p align=\"center\"><img src=\"foo.png\" /></p>\n\n# Heading",
			},
			wantHTML: `<p align="center"><img src="https://github.com/valid/module_name/raw/v1.0.0/foo.png"/></p>` + "\n" +
				`<h3 class="h1" id="readme-heading">Heading</h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading", ID: "readme-heading"},
			},
		},
		{
			name: "image link in embedded HTML with surrounding div",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<div align=\"center\"><img src=\"foo.png\" /></div>\n\n# Heading",
			},
			wantHTML: `<div align="center"><img src="https://github.com/valid/module_name/raw/v1.0.0/foo.png"/></div>` + "\n" +
				`<h3 class="h1" id="readme-heading">Heading</h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading", ID: "readme-heading"},
			},
		},
		{
			name: "image link with bad URL",
			unit: &internal.Unit{
				UnitMeta: internal.UnitMeta{
					ModuleInfo: internal.ModuleInfo{
						Version:    "v1.2.3",
						SourceInfo: source.NewGitHubInfo("https://github.com/some/<script>", "", "v1.2.3"),
					},
				},
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<div align=\"center\"><img src=\"foo.png\" /></div>\n\n# Heading",
			},
			wantHTML: `<div align="center"><img src="https://github.com/some/%3Cscript%3E/raw/v1.2.3/foo.png"/></div>` + "\n" +
				`<h3 class="h1" id="readme-heading">Heading</h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading", ID: "readme-heading"},
			},
		},
		{
			name: "body has more than one child",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				// The final newline here is important for creating the right markdown tree; do not remove it.
				Contents: `<p><img src="./foo.png"></p><p><img src="../bar.png"</p>` + "\n",
			},
			wantHTML: `<p><img src="https://github.com/valid/module_name/raw/v1.0.0/dir/sub/foo.png"/></p>` +
				`<p><img src="https://github.com/valid/module_name/raw/v1.0.0/dir/bar.png"/>` + "\n</p>",
			wantOutline: nil,
		},
		{
			name: "escaped image source",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `<img src="./images/Jupyter%20Notebook_sparkline.svg">`,
			},
			wantHTML:    `<img src="https://github.com/valid/module_name/raw/v1.0.0/images/Jupyter%20Notebook_sparkline.svg"/>`,
			wantOutline: nil,
		},
		{
			name: "relative link to local heading is prefixed with readme-",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `[Local Heading](#local-heading)` + "\n" +
					`# Local Heading`,
			},
			wantHTML: `<p><a href="#readme-local-heading" rel="nofollow">Local Heading</a></p>` + "\n" +
				`<h3 class="h1" id="readme-local-heading">Local Heading</h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Local Heading", ID: "readme-local-heading"},
			},
		},
		{
			name: "non-text content is removed from outline text",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `# Heading [![Image](file.svg)](link.html)
				`,
			},
			wantHTML: `<h3 class="h1" id="readme-heading">Heading <a href="https://github.com/valid/module_name/blob/v1.0.0/link.html" rel="nofollow"><img src="https://github.com/valid/module_name/raw/v1.0.0/file.svg" alt="Image"/></a></h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading ", ID: "readme-heading"},
			},
		},
		{
			name: "text is extracted from headings that contain only non-text nodes",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `# [![Image Text](file.svg)](link.html)
				`,
			},
			wantHTML: `<h3 class="h1" id="readme-heading"><a href="https://github.com/valid/module_name/blob/v1.0.0/link.html" rel="nofollow"><img src="https://github.com/valid/module_name/raw/v1.0.0/file.svg" alt="Image Text"/></a></h3>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Image Text", ID: "readme-heading"},
			},
		},
		{
			name: "duplicated headings ids have incremental suffix",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "# Heading\n## Heading\n## Heading",
			},
			wantHTML: `<h3 class="h1" id="readme-heading">Heading</h3>` + "\n" +
				`<h4 class="h2" id="readme-heading-1">Heading</h4>` + "\n" +
				`<h4 class="h2" id="readme-heading-2">Heading</h4>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading", ID: "readme-heading"},
				{Level: 2, Text: "Heading", ID: "readme-heading-1"},
				{Level: 2, Text: "Heading", ID: "readme-heading-2"},
			},
		},
		{
			name: "only letters and numbers are preserved in ids",
			unit: unit,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "# Heading 😎\n## 👾\n## Heading 🚀",
			},
			wantHTML: `<h3 class="h1" id="readme-heading">Heading 😎</h3>` + "\n" +
				`<h4 class="h2" id="readme-heading-1">👾</h4>` + "\n" +
				`<h4 class="h2" id="readme-heading-2">Heading 🚀</h4>`,
			wantOutline: []*Heading{
				{Level: 1, Text: "Heading 😎", ID: "readme-heading"},
				{Level: 2, Text: "👾", ID: "readme-heading-1"},
				{Level: 2, Text: "Heading 🚀", ID: "readme-heading-2"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.unit.Readme = test.readme
			readme, err := ProcessReadme(ctx, test.unit)
			if err != nil {
				t.Fatal(err)
			}
			gotHTML := strings.TrimSpace(readme.HTML.String())
			if diff := cmp.Diff(test.wantHTML, gotHTML); diff != "" {
				t.Errorf("Readme(%v) html mismatch (-want +got):\n%s", test.unit.UnitMeta, diff)
			}
			if diff := cmp.Diff(test.wantOutline, readme.Outline); diff != "" {
				t.Errorf("Readme(%v) outline mismatch (-want +got):\n%s", test.unit.UnitMeta, diff)
			}
		})
	}
}

func TestReadmeLinks(t *testing.T) {
	ctx := experiment.NewContext(context.Background())
	unit := sample.UnitEmpty(sample.PackagePath, sample.ModulePath, sample.VersionString)
	for _, test := range []struct {
		name     string
		contents string
		want     []link
	}{
		{
			name: "no links",
			contents: `
				# Heading
				Some stuff.
			`,
			want: nil,
		},
		{
			name: "simple links",
			contents: `
				# Heading
				Some stuff.

				## Links
				Here are some links:

				- [a](http://a)
				- [b](http://b)

				Whatever.

				1. [c](http://c)
			`,
			want: []link{
				{"http://a", "a"},
				{"http://b", "b"},
				{"http://c", "c"},
			},
		},
		{
			name: "ignore links not in a list",
			contents: `
				# Links
				Try [a](http://a).
				- [b](http://b)
			`,
			want: []link{
				{"http://b", "b"},
			},
		},
		{
			name: "ignore extra text",
			contents: `
				# Links
				- Try [a](http://a).
				- [b](http://b)
			`,
			want: []link{
				{"http://b", "b"},
			},
		},
		{
			name: "ignore sub-headings",
			contents: `
				# Links
				- [a](http://a)
				## Sub
				- [b](http://b)
				## Links
				- [c](http://c)
			`,
			want: []link{{"http://a", "a"}},
		},
		{
			name: "ignore nested links",
			contents: `
				# Links
				- [a](http://a)
				   - [b](http://b)
				- [c](http://c)
			`,
			want: []link{
				{"http://a", "a"},
				{"http://c", "c"},
			},
		},
		{
			name: "two links sections",
			contents: `
				# Links
				- [a](http://a)
				# Links
				- [b](http://b)
			`,
			want: []link{{"http://a", "a"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit.Readme = &internal.Readme{
				Filepath: "README.md",
				Contents: unindent(test.contents),
			}
			got, err := ProcessReadme(ctx, unit)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got.Links); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// unindent removes indentation from s. It assumes that s starts with an initial
// newline followed by one or more indented lines.
func unindent(s string) string {
	i := strings.IndexFunc(s, func(r rune) bool { return !unicode.IsSpace(r) })
	if i < 0 {
		return s
	}
	indent := s[:i]
	return strings.ReplaceAll(s, indent, "\n")[1:]
}

func TestUnindent(t *testing.T) {
	s := `
		a
		 - b
		c
	`
	got := unindent(s)
	want := "a\n - b\nc\n\t"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
