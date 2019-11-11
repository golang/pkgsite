// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/discovery/internal/version"
)

func TestFetchOverviewDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *OverviewDetails
	}{
		name:    "want expected overview details",
		version: sample.Version(),
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

	if err := testDB.InsertVersion(ctx, tc.version); err != nil {
		t.Fatal(err)
	}

	got := fetchOverviewDetails(ctx, testDB, &tc.version.VersionInfo, sample.LicenseMetadata)
	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchOverviewDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name string
		vi   *internal.VersionInfo
		want template.HTML
	}{
		{
			name: "valid markdown readme",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1)."),
			},
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name: "not markdown readme",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.rst",
				ReadmeContents: []byte("This package collects pithy sayings.\n\n" +
					"It's part of a demonstration of\n" +
					"[package versioning in Go](https://research.swtch.com/vgo1)."),
			},
			want: template.HTML("<pre class=\"readme\">This package collects pithy sayings.\n\nIt&#39;s part of a demonstration of\n[package versioning in Go](https://research.swtch.com/vgo1).</pre>"),
		},
		{
			name: "empty readme",
			vi:   &internal.VersionInfo{},
			want: "",
		},
		{
			name: "sanitized readme",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README",
				ReadmeContents: []byte(`<a onblur="alert(secret)" href="http://www.google.com">Google</a>`),
			},
			want: template.HTML(`<pre class="readme">&lt;a onblur=&#34;alert(secret)&#34; href=&#34;http://www.google.com&#34;&gt;Google&lt;/a&gt;</pre>`),
		},
		{
			name: "relative image markdown is made absolute for GitHub",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
				SourceInfo:     source.NewGitHubInfo("http://github.com/golang/go", "", "master"),
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/go/master/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "relative image markdown is made absolute for GitLab",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Gitaly benchmark timings.](doc/img/rugged-new-timings.png)"),
				SourceInfo:     source.NewGitLabInfo("http://gitlab.com/gitlab-org/gitaly", "", "v1.0.0"),
			},
			want: template.HTML("<p><img src=\"http://gitlab.com/gitlab-org/gitaly/raw/v1.0.0/doc/img/rugged-new-timings.png\" alt=\"Gitaly benchmark timings.\"/></p>\n"),
		},
		{
			name: "relative image markdown is left alone for unknown origins",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
			},
			want: template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "module versions are referenced in relative images",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Hugo logo](doc/logo.png)"),
				Version:        "v0.56.3",
				VersionType:    version.TypeRelease,
				SourceInfo:     source.NewGitHubInfo("http://github.com/gohugoio/hugo", "", "v0.56.3"),
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/gohugoio/hugo/v0.56.3/doc/logo.png\" alt=\"Hugo logo\"/></p>\n"),
		},
		{
			name: "image URLs relative to README directory",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "dir/sub/README.md",
				ReadmeContents: []byte("![alt](img/thing.png)"),
				Version:        "v1.2.3",
				VersionType:    version.TypeRelease,
				SourceInfo:     source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
			},
			want: template.HTML(`<p><img src="https://raw.githubusercontent.com/some/repo/v1.2.3/dir/sub/img/thing.png" alt="alt"/></p>` + "\n"),
		},
		{
			name: "non-image links relative to README directory",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "dir/sub/README.md",
				ReadmeContents: []byte("[something](doc/thing.md)"),
				Version:        "v1.2.3",
				VersionType:    version.TypeRelease,
				SourceInfo:     source.NewGitHubInfo("https://github.com/some/repo", "", "v1.2.3"),
			},
			want: template.HTML(`<p><a href="https://github.com/some/repo/blob/v1.2.3/dir/sub/doc/thing.md" rel="nofollow">something</a></p>` + "\n"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML(tc.vi)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%v) mismatch (-want +got):\n%s", tc.vi, diff)
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
