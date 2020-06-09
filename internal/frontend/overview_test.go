// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
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
			ReadMe:          template.HTML("<p>readme</p>\n"),
			ReadMeSource:    "github.com/valid_module_name@v1.0.0/README.md",
			ModuleURL:       "/mod/github.com/valid_module_name@v1.0.0",
			Redistributable: true,
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertModule(ctx, tc.module); err != nil {
		t.Fatal(err)
	}

	got := constructOverviewDetails(&tc.module.LegacyModuleInfo, true, true)
	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("constructOverviewDetails(%q, %q) mismatch (-want +got):\n%s", tc.module.LegacyPackages[0].Path, tc.module.Version, diff)
	}
}

func TestConstructPackageOverviewDetailsNew(t *testing.T) {
	for _, test := range []struct {
		name           string
		vdir           *internal.VersionedDirectory
		versionedLinks bool
		want           *OverviewDetails
	}{
		{
			name: "redistributable",
			vdir: &internal.VersionedDirectory{
				DirectoryNew: internal.DirectoryNew{
					Path:              "github.com/u/m/p",
					IsRedistributable: true,
				},
				LegacyModuleInfo: *sample.LegacyModuleInfo("github.com/u/m", "v1.2.3"),
			},
			versionedLinks: true,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m@v1.2.3",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           template.HTML("<p>readme</p>\n"),
				ReadMeSource:     "github.com/u/m@v1.2.3/README.md",
				Redistributable:  true,
			},
		},
		{
			name: "unversioned",
			vdir: &internal.VersionedDirectory{
				DirectoryNew: internal.DirectoryNew{
					Path:              "github.com/u/m/p",
					IsRedistributable: true,
				},
				LegacyModuleInfo: *sample.LegacyModuleInfo("github.com/u/m", "v1.2.3"),
			},
			versionedLinks: false,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           template.HTML("<p>readme</p>\n"),
				ReadMeSource:     "github.com/u/m@v1.2.3/README.md",
				Redistributable:  true,
			},
		},
		{
			name: "non-redistributable",
			vdir: &internal.VersionedDirectory{
				DirectoryNew: internal.DirectoryNew{
					Path:              "github.com/u/m/p",
					IsRedistributable: false,
				},
				LegacyModuleInfo: *sample.LegacyModuleInfo("github.com/u/m", "v1.2.3"),
			},
			versionedLinks: true,
			want: &OverviewDetails{
				ModulePath:       "github.com/u/m",
				ModuleURL:        "/mod/github.com/u/m@v1.2.3",
				RepositoryURL:    "https://github.com/u/m",
				PackageSourceURL: "https://github.com/u/m/tree/v1.2.3/p",
				ReadMe:           "",
				ReadMeSource:     "",
				Redistributable:  false,
			},
		},
	} {
		got := fetchPackageOverviewDetailsNew(test.vdir, test.versionedLinks)
		if diff := cmp.Diff(test.want, got); diff != "" {
			t.Errorf("%s: mismatch (-want +got):\n%s", test.name, diff)
		}
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name string
		mi   *internal.LegacyModuleInfo
		want template.HTML
	}{
		{
			name: "valid markdown readme",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.md",
				LegacyReadmeContents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name: "valid markdown readme with alternative case and extension",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.MARKDOWN",
				LegacyReadmeContents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name: "not markdown readme",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.rst",
				LegacyReadmeContents: "This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1).",
			},
			want: template.HTML("<pre class=\"readme\">This package collects pithy sayings.\n\nIt&#39;s part of a demonstration of\n[package versioning in Go](https://research.swtch.com/vgo1).</pre>"),
		},
		{
			name: "empty readme",
			mi:   &internal.LegacyModuleInfo{},
			want: "",
		},
		{
			name: "sanitized readme",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README",
				LegacyReadmeContents: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`,
			},
			want: template.HTML(`<pre class="readme">&lt;a onblur=&#34;alert(secret)&#34; href=&#34;http://www.google.com&#34;&gt;Google&lt;/a&gt;</pre>`),
		},
		{
			name: "relative image markdown is made absolute for GitHub",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.md",
				LegacyReadmeContents: "![Go logo](doc/logo.png)",
				SourceInfo:           source.NewGitHubInfo("http://github.com/golang/go", "", "master"),
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/go/master/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "relative image markdown is made absolute for GitLab",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.md",
				LegacyReadmeContents: "![Gitaly benchmark timings.](doc/img/rugged-new-timings.png)",
				SourceInfo:           source.NewGitLabInfo("http://gitlab.com/gitlab-org/gitaly", "", "v1.0.0"),
			},
			want: template.HTML("<p><img src=\"http://gitlab.com/gitlab-org/gitaly/raw/v1.0.0/doc/img/rugged-new-timings.png\" alt=\"Gitaly benchmark timings.\"/></p>\n"),
		},
		{
			name: "relative image markdown is left alone for unknown origins",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.md",
				LegacyReadmeContents: "![Go logo](doc/logo.png)",
			},
			want: template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "module versions are referenced in relative images",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "README.md",
				LegacyReadmeContents: "![Hugo logo](doc/logo.png)",
				Version:              "v0.56.3",
				VersionType:          version.TypeRelease,
				SourceInfo:           source.NewGitHubInfo("http://github.com/gohugoio/hugo", "", "v0.56.3"),
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/gohugoio/hugo/v0.56.3/doc/logo.png\" alt=\"Hugo logo\"/></p>\n"),
		},
		{
			name: "image URLs relative to README directory",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "dir/sub/README.md",
				LegacyReadmeContents: "![alt](img/thing.png)",
				Version:              "v1.2.3",
				VersionType:          version.TypeRelease,
				SourceInfo:           source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
			},
			want: template.HTML(`<p><img src="https://raw.githubusercontent.com/some/repo/v1.2.3/dir/sub/img/thing.png" alt="alt"/></p>` + "\n"),
		},
		{
			name: "non-image links relative to README directory",
			mi: &internal.LegacyModuleInfo{
				LegacyReadmeFilePath: "dir/sub/README.md",
				LegacyReadmeContents: "[something](doc/thing.md)",
				Version:              "v1.2.3",
				VersionType:          version.TypeRelease,
				SourceInfo:           source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
			},
			want: template.HTML(`<p><a href="https://github.com/some/repo/blob/v1.2.3/dir/sub/doc/thing.md" rel="nofollow">something</a></p>` + "\n"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML(tc.mi)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%v) mismatch (-want +got):\n%s", tc.mi, diff)
			}
		})
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
		got := packageSubdir(test.pkgPath, test.modulePath)
		if got != test.want {
			t.Errorf("packageSubdir(%q, %q) = %q, want %q", test.pkgPath, test.modulePath, got, test.want)
		}
	}
}
