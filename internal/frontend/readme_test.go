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
	"golang.org/x/discovery/internal/sample"
)

func TestFetchReadMeDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *ReadMeDetails
	}{
		name:    "want expected overview details",
		version: sample.Version(),
		wantDetails: &ReadMeDetails{
			ModulePath: sample.ModulePath,
			ReadMe:     template.HTML("<p>readme</p>\n"),
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, sample.Licenses); err != nil {
		t.Fatal(err)
	}

	got, err := fetchReadMeDetails(ctx, testDB, &tc.version.VersionInfo)
	if err != nil {
		t.Fatalf("fetchReadMeDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetails)
	}

	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchReadMeDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
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
			want: template.HTML("<pre class=\"readme\"></pre>"),
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
				RepositoryURL:  "http://github.com/golang/go",
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/go/master/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "relative image markdown is left alone for unknown origins",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
				RepositoryURL:  "http://example.com/golang/go",
			},
			want: template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "valid markdown readme, invalid repositoryURL",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
				RepositoryURL:  ":",
			},
			want: template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "go module versions are converted to go release tags",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
				Version:        "v1.12.0",
				VersionType:    internal.VersionTypeRelease,
				RepositoryURL:  "http://github.com/golang/go",
				ModulePath:     "std",
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/go/go1.12/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name: "module release versions are referenced in relative images",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Hugo logo](doc/logo.png)"),
				Version:        "v0.56.3",
				VersionType:    internal.VersionTypeRelease,
				RepositoryURL:  "http://github.com/gohugoio/hugo",
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/gohugoio/hugo/v0.56.3/doc/logo.png\" alt=\"Hugo logo\"/></p>\n"),
		},
		{
			name: "module prerelease versions are referenced in relative images",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Hugo logo](doc/logo.png)"),
				Version:        "v0.56.3-pre",
				VersionType:    internal.VersionTypePrerelease,
				RepositoryURL:  "http://github.com/gohugoio/hugo",
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/gohugoio/hugo/v0.56.3-pre/doc/logo.png\" alt=\"Hugo logo\"/></p>\n"),
		},
		{
			name: "module pseudoversions are converted to git refs in relative images",
			vi: &internal.VersionInfo{
				ReadmeFilePath: "README.md",
				ReadmeContents: []byte("![Go logo](doc/logo.png)"),
				Version:        "v0.0.0-20190306220234-b354f8bf4d9e",
				RepositoryURL:  "http://github.com/golang/sys",
				VersionType:    internal.VersionTypePseudo,
			},
			want: template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/sys/b354f8bf4d9e/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
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
