// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestBlackfridayReadmeHTML(t *testing.T) {
	ctx := context.Background()
	aModule := &internal.ModuleInfo{
		Version:    "v1.2.3",
		SourceInfo: source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
	}
	tests := []struct {
		name         string
		mi           *internal.ModuleInfo
		readme       *internal.Readme
		want         string
		wantUnitPage string
	}{
		{
			name: "valid markdown readme",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: "<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>`,
		},
		{
			name: "valid markdown readme with alternative case and extension",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README.MARKDOWN",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: "<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>`,
		},
		{
			name: "valid markdown readme with CRLF",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "This package collects pithy sayings.\r\n\r\n" +
					"- It's part of a demonstration of\r\n" +
					"- [package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: "<p>This package collects pithy sayings.</p>\n\n" +
				"<ul>\n" +
				"<li>It’s part of a demonstration of</li>\n" +
				`<li><a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</li>` + "\n" +
				"</ul>",
		},
		{
			name: "not markdown readme",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README.rst",
				Contents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: "<pre class=\"readme\">This package collects pithy sayings.\n\nIt&#39;s part of a demonstration of\n[package versioning in Go](https://research.swtch.com/vgo1).</pre>",
		},
		{
			name: "empty readme",
			mi:   &internal.ModuleInfo{},
			want: "",
		},
		{
			name: "sanitized readme",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README",
				Contents: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`,
			},
			want: `<pre class="readme">&lt;a onblur=&#34;alert(secret)&#34; href=&#34;http://www.google.com&#34;&gt;Google&lt;/a&gt;</pre>`,
		},
		{
			name: "relative image markdown is made absolute for GitHub",
			mi: &internal.ModuleInfo{
				SourceInfo: source.NewGitHubInfo("http://github.com/golang/go", "", "master"),
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Go logo](doc/logo.png)",
			},
			want: `<p><img src="http://github.com/golang/go/raw/master/doc/logo.png" alt="Go logo"/></p>`,
		},
		{
			name: "relative image markdown is left alone for unknown origins",
			mi:   &internal.ModuleInfo{},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Go logo](doc/logo.png)",
			},
			want: `<p><img src="doc/logo.png" alt="Go logo"/></p>`,
		},
		{
			name: "module versions are referenced in relative images",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "![Hugo logo](doc/logo.png)",
			},
			want: `<p><img src="https://github.com/some/repo/raw/v1.2.3/doc/logo.png" alt="Hugo logo"/></p>`,
		},
		{
			name: "image URLs relative to README directory",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				Contents: "![alt](img/thing.png)",
			},
			want: `<p><img src="https://github.com/some/repo/raw/v1.2.3/dir/sub/img/thing.png" alt="alt"/></p>`,
		},
		{
			name: "non-image links relative to README directory",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				Contents: "[something](doc/thing.md)",
			},
			want: `<p><a href="https://github.com/some/repo/blob/v1.2.3/dir/sub/doc/thing.md" rel="nofollow">something</a></p>`,
		},
		{
			name: "image link in embedded HTML",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<img src=\"resources/logoSmall.png\" />\n\n# Heading\n",
			},
			want:         `<p><img src="https://github.com/some/repo/raw/v1.2.3/resources/logoSmall.png"/></p>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
			wantUnitPage: `<p><img src="https://github.com/some/repo/raw/v1.2.3/resources/logoSmall.png"/></p>` + "\n\n" + `<h1 id="readme-heading">Heading</h1>`,
		},
		{
			name: "image link in embedded HTML with surrounding p tag",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<p align=\"center\"><img src=\"foo.png\" /></p>\n\n# Heading",
			},
			want:         `<p align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></p>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
			wantUnitPage: `<p align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></p>` + "\n\n" + `<h1 id="readme-heading">Heading</h1>`,
		},
		{
			name: "image link in embedded HTML with surrounding div",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<div align=\"center\"><img src=\"foo.png\" /></div>\n\n# Heading",
			},
			want:         `<div align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
			wantUnitPage: `<div align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="readme-heading">Heading</h1>`,
		},
		{
			name: "image link with bad URL",
			mi: &internal.ModuleInfo{
				Version:    "v1.2.3",
				SourceInfo: source.NewGitHubInfo("https://github.com/some/<script>", "", "v1.2.3"),
			},
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<div align=\"center\"><img src=\"foo.png\" /></div>\n\n# Heading",
			},
			want:         `<div align="center"><img src="https://github.com/some/%3Cscript%3E/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
			wantUnitPage: `<div align="center"><img src="https://github.com/some/%3Cscript%3E/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="readme-heading">Heading</h1>`,
		},
		{
			name: "body has more than one child",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "dir/sub/README.md",
				// The final newline here is important for creating the right markdown tree; do not remove it.
				Contents: `<p><img src="./foo.png"></p><p><img src="../bar.png"</p>` + "\n",
			},
			want: `<p><img src="https://github.com/some/repo/raw/v1.2.3/dir/sub/foo.png"/></p><p><img src="https://github.com/some/repo/raw/v1.2.3/dir/bar.png"/></p>`,
		},
		{
			name: "escaped image source",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `<img src="./images/Jupyter%20Notebook_sparkline.svg">`,
			},
			want: `<p><img src="https://github.com/some/repo/raw/v1.2.3/images/Jupyter%20Notebook_sparkline.svg"/></p>`,
		},
		{
			name: "relative link to local heading is prefixed with readme-",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `[Local Heading](#heading-id)`,
			},
			want: `<p><a href="#readme-heading-id" rel="nofollow">Local Heading</a></p>`,
		},
		{
			name: "absolute link to blob",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `<img src="https://github.com/foo/bar/blob/master/logo.svg">`,
			},
			want: `<p><img src="https://github.com/foo/bar/raw/master/logo.svg"/></p>`,
		},
		{
			name: "absolute link not to blob",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: `<img src="https://github.com/foo/bar/bloob/master/logo.svg">`,
			},
			want: `<p><img src="https://github.com/foo/bar/bloob/master/logo.svg"></p>`,
		},
	}
	checkReadme := func(ctx context.Context, t *testing.T, mi *internal.ModuleInfo, readme *internal.Readme, want string) {
		hgot, err := LegacyReadmeHTML(ctx, mi, readme)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(hgot.String())
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("LegacyReadmeHTML(%v) mismatch (-want +got):\n%s", mi, diff)
		}
	}
	for _, test := range tests {
		if test.wantUnitPage == "" {
			test.wantUnitPage = test.want
		}
		checkReadme(ctx, t, test.mi, test.readme, test.wantUnitPage)
	}
}

func TestTrimmedEscapedPath(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"a.png", "a.png"},
		{" a.png   ", "a.png"},
		{"a b.png", "a%20b.png"},
		{" a b.png ", "a%20b.png"},
		{".a/b.gif", ".a/b.gif"},
	} {
		u, err := url.Parse(test.in)
		if err != nil {
			t.Fatal(err)
		}
		got := trimmedEscapedPath(u)
		if got != test.want {
			t.Errorf("escapePath(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestPackageSubdir(t *testing.T) {
	for _, test := range []struct {
		pkgPath, modulePath string
		want                string
	}{
		// package at module root
		{"github.com/pkg/errors", "github.com/pkg/errors", ""},
		// package inside module
		{"github.com/google/go-cmp/cmp", "github.com/google/go-cmp", "cmp"},
		// stdlib package
		{"context", stdlib.ModulePath, "context"},
	} {
		got := internal.Suffix(test.pkgPath, test.modulePath)
		if got != test.want {
			t.Errorf("internal.Suffix(%q, %q) = %q, want %q", test.pkgPath, test.modulePath, got, test.want)
		}
	}
}
