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
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestFetchOverviewDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		module      *internal.Module
		wantDetails *OverviewDetails
	}{
		name:   "want expected overview details",
		module: sample.DefaultModule(),
		wantDetails: &OverviewDetails{
			ModulePath:      sample.ModulePath,
			RepositoryURL:   sample.RepositoryURL,
			ReadMe:          testconversions.MakeHTMLForTest("<p>readme</p>\n"),
			ReadMeSource:    sample.ModulePath + "@v1.0.0/README.md",
			ModuleURL:       "/mod/" + sample.ModulePath + "@v1.0.0",
			Redistributable: true,
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertModule(ctx, tc.module); err != nil {
		t.Fatal(err)
	}

	readme := &internal.Readme{Filepath: tc.module.LegacyReadmeFilePath, Contents: tc.module.LegacyReadmeContents}
	got, err := constructOverviewDetails(ctx, &tc.module.ModuleInfo, readme, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(tc.wantDetails, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
		t.Errorf("constructOverviewDetails(%q, %q) mismatch (-want +got):\n%s",
			tc.module.LegacyPackages[0].Path, tc.module.Version, diff)
	}
}

func TestPackageOverviewDetails(t *testing.T) {
	for _, test := range []struct {
		name           string
		dir            *internal.Unit
		versionedLinks bool
		want           *OverviewDetails
	}{
		{
			name: "redistributable",
			dir: &internal.Unit{
				UnitMeta: *sample.UnitMeta(
					"github.com/u/m/p",
					"github.com/u/m",
					"v1.2.3",
					"",
					true,
				),
				Readme: &internal.Readme{
					Filepath: "README.md",
					Contents: "readme",
				},
			},
			versionedLinks: true,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m@v1.2.3",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           testconversions.MakeHTMLForTest("<p>readme</p>\n"),
				ReadMeSource:     "github.com/u/m@v1.2.3/README.md",
				Redistributable:  true,
			},
		},
		{
			name: "unversioned",
			dir: &internal.Unit{
				UnitMeta: *sample.UnitMeta(
					"github.com/u/m/p",
					"github.com/u/m",
					"v1.2.3",
					"",
					true,
				),
				Readme: &internal.Readme{
					Filepath: "README.md",
					Contents: "readme",
				},
			},
			versionedLinks: false,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           testconversions.MakeHTMLForTest("<p>readme</p>\n"),
				ReadMeSource:     "github.com/u/m@v1.2.3/README.md",
				Redistributable:  true,
			},
		},
		{
			name: "non-redistributable",
			dir: &internal.Unit{
				UnitMeta: *sample.UnitMeta(
					"github.com/u/m/p",
					"github.com/u/m",
					"v1.2.3",
					"",
					false,
				),
			},
			versionedLinks: true,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m@v1.2.3",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           safehtml.HTML{},
				ReadMeSource:     "",
				Redistributable:  false,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)
			m := sample.Module(
				test.dir.ModulePath,
				test.dir.Version,
				internal.Suffix(test.dir.Path, test.dir.ModulePath))
			m.Units[1].IsRedistributable = test.dir.IsRedistributable
			m.SourceInfo = source.NewGitHubInfo("https://"+test.dir.ModulePath, "", test.dir.Version)

			ctx := context.Background()
			if err := testDB.InsertModule(ctx, m); err != nil {
				t.Fatal(err)
			}
			pi := &internal.UnitMeta{
				Path:              test.dir.Path,
				ModulePath:        test.dir.ModulePath,
				Version:           test.dir.Version,
				IsRedistributable: test.dir.IsRedistributable,
				Name:              test.dir.Name,
				CommitTime:        test.dir.CommitTime,
				SourceInfo:        m.SourceInfo,
			}
			got, err := fetchPackageOverviewDetails(ctx, testDB, pi, test.versionedLinks)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadmeHTML(t *testing.T) {
	ctx := context.Background()
	aModule := &internal.ModuleInfo{
		Version:    "v1.2.3",
		SourceInfo: source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
	}
	for _, tc := range []struct {
		name   string
		mi     *internal.ModuleInfo
		readme *internal.Readme
		want   string
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
			want: `<p><img src="https://github.com/some/repo/raw/v1.2.3/resources/logoSmall.png"/></p>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
		},
		{
			name: "image link in embedded HTML with surrounding p tag",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<p align=\"center\"><img src=\"foo.png\" /></p>\n\n# Heading",
			},
			want: `<p align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></p>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
		},
		{
			name: "image link in embedded HTML with surrounding div",
			mi:   aModule,
			readme: &internal.Readme{
				Filepath: "README.md",
				Contents: "<div align=\"center\"><img src=\"foo.png\" /></div>\n\n# Heading",
			},
			want: `<div align="center"><img src="https://github.com/some/repo/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
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
			want: `<div align="center"><img src="https://github.com/some/%3Cscript%3E/raw/v1.2.3/foo.png"/></div>` + "\n\n" + `<h1 id="heading">Heading</h1>`,
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			hgot, err := ReadmeHTML(ctx, tc.mi, tc.readme)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(hgot.String())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%v) mismatch (-want +got):\n%s", tc.mi, diff)
			}
		})
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
