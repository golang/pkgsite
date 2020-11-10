// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestReadme(t *testing.T) {
	ctx := experiment.NewContext(context.Background(), internal.ExperimentGoldmark)
	unit := sample.UnitEmpty(sample.PackagePath, sample.ModulePath, sample.VersionString)
	for _, tc := range []struct {
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
				UnitMeta: internal.UnitMeta{SourceInfo: source.NewGitHubInfo("http://github.com/golang/go", "", "master")},
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Go logo](doc/logo.png)",
			},
			wantHTML:    `<p><img src="http://github.com/golang/go/raw/master/doc/logo.png" alt="Go logo"/></p>`,
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
					Version:    "v1.2.3",
					SourceInfo: source.NewGitHubInfo("https://github.com/some/<script>", "", "v1.2.3"),
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.unit.Readme = tc.readme
			html, gotOutline, err := Readme(ctx, tc.unit)
			if err != nil {
				t.Fatal(err)
			}
			gotHTML := strings.TrimSpace(html.String())
			if diff := cmp.Diff(tc.wantHTML, gotHTML); diff != "" {
				t.Errorf("Readme(%v) html mismatch (-want +got):\n%s", tc.unit.UnitMeta, diff)
			}
			if diff := cmp.Diff(tc.wantOutline, gotOutline); diff != "" {
				t.Errorf("Readme(%v) outline mismatch (-want +got):\n%s", tc.unit.UnitMeta, diff)
			}
		})
	}
}
