// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/testing/htmlcheck"
	"golang.org/x/discovery/internal/testing/pagecheck"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/net/html"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

func TestHTMLInjection(t *testing.T) {
	s, err := NewServer(testDB, nil, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

// TestServer checks the contents of served pages by looking for
// strings and elements in the parsed HTML response body.
//
// Other than search and static content, our pages vary along five dimensions:
//
// 1. module / package / directory
// 2. stdlib / other (since the standard library is a special case in several ways)
// 3. redistributable / non-redistributable
// 4. versioned / unversioned URL (whether the URL for the page contains "@version")
// 5. the tab (overview / doc / imports / ...)
//
// We aim to test all combinations of these.

const pseudoVersion = "v0.0.0-20190101-123456789012"

type testModule struct {
	path            string
	redistributable bool
	versions        []string
	packages        []testPackage
}

type testPackage struct {
	name string
	path string
	doc  string
}

var testModules = []testModule{
	{
		// An ordinary module, with three versions.
		path:            sample.ModulePath,
		redistributable: true,
		versions:        []string{"v1.0.0", "v0.9.0", pseudoVersion},
		packages: []testPackage{
			{
				name: "foo",
				path: sample.ModulePath + "/foo",
			},
			{
				name: "hello",
				path: sample.ModulePath + "/foo/directory/hello",
				doc:  `<a href="/pkg/io#Writer">io.Writer</a>`,
			},
		},
	},
	{
		// A non-redistributable module.
		path:            "github.com/non_redistributable",
		redistributable: false,
		versions:        []string{"v1.0.0"},
		packages: []testPackage{
			{
				name: "bar",
				path: "github.com/non_redistributable/bar",
			},
		},
	},
	{
		// A module whose latest version is a pseudoversion.
		path:            "github.com/pseudo",
		redistributable: true,
		versions:        []string{pseudoVersion},
		packages: []testPackage{
			{
				name: "baz",
				path: "github.com/pseudo/dir/baz",
			},
		},
	},
	{
		// A module whose latest version is has "+incompatible".
		path:            "github.com/incompatible",
		redistributable: true,
		versions:        []string{"v1.0.0+incompatible"},
		packages: []testPackage{
			{
				name: "inc",
				path: "github.com/incompatible/dir/inc",
			},
		},
	},
	{
		// A standard library module.
		path:            "std",
		redistributable: true,
		versions:        []string{"v1.13.0"},
		packages: []testPackage{
			{
				name: "main",
				path: "cmd/go",
			},
		},
	},
}

func insertTestModules(ctx context.Context, t *testing.T, mods []testModule) {
	for _, mod := range mods {
		var ps []*internal.Package
		for _, pkg := range mod.packages {
			p := sample.Package()
			p.Name = pkg.name
			p.Path = pkg.path
			if pkg.doc != "" {
				p.DocumentationHTML = pkg.doc
			}
			if !mod.redistributable {
				p.Licenses = nil
			}
			ps = append(ps, p)
		}
		for _, ver := range mod.versions {
			v := sample.Version()
			v.ModulePath = mod.path
			v.Version = ver
			v.SourceInfo = source.NewGitHubInfo(sample.RepositoryURL, "", ver)
			v.Packages = ps
			if err := testDB.InsertVersion(ctx, v); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	insertTestModules(ctx, t, testModules)

	s, err := NewServer(testDB, nil, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)
	handler := middleware.LatestVersion(s.LatestVersion)(mux)

	var (
		in   = htmlcheck.In
		inAt = htmlcheck.InAt
		text = htmlcheck.HasText
		attr = htmlcheck.HasAttr

		// href checks for an exact match in an href attribute.
		href = func(val string) htmlcheck.Checker {
			return attr("href", "^"+regexp.QuoteMeta(val)+"$")
		}
	)

	pkgV100 := &pagecheck.Page{
		Title:            "package foo",
		ModulePath:       "github.com/valid_module_name",
		Version:          "v1.0.0",
		Suffix:           "foo",
		IsLatest:         true,
		LatestLink:       "/github.com/valid_module_name@v1.0.0/foo",
		LicenseType:      "MIT",
		PackageURLFormat: "/github.com/valid_module_name%s/foo",
		ModuleURL:        "/mod/github.com/valid_module_name",
	}
	p9 := *pkgV100
	p9.Version = "v0.9.0"
	p9.IsLatest = false
	pkgV090 := &p9

	pp := *pkgV100
	pp.Version = pseudoVersion
	pp.FormattedVersion = "v0.0.0 (20190101-123456789012)"
	pp.IsLatest = false
	pkgPseudo := &pp

	pkgInc := &pagecheck.Page{
		Title:            "package inc",
		ModulePath:       "github.com/incompatible",
		Version:          "v1.0.0+incompatible",
		Suffix:           "dir/inc",
		IsLatest:         true,
		LatestLink:       "/github.com/incompatible@v1.0.0+incompatible/dir/inc",
		LicenseType:      "MIT",
		PackageURLFormat: "/github.com/incompatible%s/dir/inc",
		ModuleURL:        "/mod/github.com/incompatible",
	}

	pkgNonRedist := &pagecheck.Page{
		Title:            "package bar",
		ModulePath:       "github.com/non_redistributable",
		Version:          "v1.0.0",
		Suffix:           "bar",
		IsLatest:         true,
		LatestLink:       "/github.com/non_redistributable@v1.0.0/bar",
		LicenseType:      "",
		PackageURLFormat: "/github.com/non_redistributable%s/bar",
		ModuleURL:        "/mod/github.com/non_redistributable",
	}
	cmdGo := &pagecheck.Page{
		Title:            "command go",
		ModulePath:       "std",
		Suffix:           "cmd/go",
		Version:          "go1.13",
		LicenseType:      "MIT",
		IsLatest:         true,
		LatestLink:       "/cmd/go@go1.13",
		PackageURLFormat: "/cmd/go%s",
		ModuleURL:        "/std",
	}
	mod := &pagecheck.Page{
		ModulePath:  "github.com/valid_module_name",
		Title:       "module github.com/valid_module_name",
		ModuleURL:   "/mod/github.com/valid_module_name",
		Version:     "v1.0.0",
		LicenseType: "MIT",
		IsLatest:    true,
		LatestLink:  "/mod/github.com/valid_module_name@v1.0.0",
	}
	mp := *mod
	mp.Version = pseudoVersion
	mp.FormattedVersion = "v0.0.0 (20190101-123456789012)"
	mp.IsLatest = false
	modPseudo := &mp

	mod2 := &pagecheck.Page{
		ModulePath:       "github.com/pseudo",
		Title:            "module github.com/pseudo",
		ModuleURL:        "/mod/github.com/pseudo",
		LatestLink:       "/mod/github.com/pseudo@" + pseudoVersion,
		Version:          pseudoVersion,
		FormattedVersion: mp.FormattedVersion,
		LicenseType:      "MIT",
		IsLatest:         true,
	}
	dirPseudo := &pagecheck.Page{
		ModulePath:       "github.com/pseudo",
		Title:            "directory github.com/pseudo/dir",
		ModuleURL:        "/mod/github.com/pseudo",
		LatestLink:       "/mod/github.com/pseudo@" + pseudoVersion + "/dir",
		Suffix:           "dir",
		Version:          pseudoVersion,
		FormattedVersion: mp.FormattedVersion,
		LicenseType:      "MIT",
		IsLatest:         true,
		PackageURLFormat: "/github.com/pseudo%s/dir",
	}

	std := &pagecheck.Page{
		Title:       "Standard library",
		ModulePath:  "std",
		Version:     "go1.13",
		LicenseType: "MIT",
		ModuleURL:   "/std",
		IsLatest:    true,
		LatestLink:  "/std@go1.13",
	}

	dir := &pagecheck.Page{
		Title:            "directory github.com/valid_module_name/foo/directory",
		ModulePath:       "github.com/valid_module_name",
		Version:          "v1.0.0",
		Suffix:           "foo/directory",
		LicenseType:      "MIT",
		ModuleURL:        "/mod/github.com/valid_module_name",
		PackageURLFormat: "/github.com/valid_module_name%s/foo/directory",
	}

	dirCmd := &pagecheck.Page{
		Title:            "directory cmd",
		ModulePath:       "std",
		Version:          "go1.13",
		Suffix:           "cmd",
		LicenseType:      "MIT",
		ModuleURL:        "/std",
		PackageURLFormat: "/cmd%s",
	}

	const (
		versioned   = true
		unversioned = false
	)

	pkgSuffix := strings.TrimPrefix(sample.PackagePath, sample.ModulePath+"/")
	for _, tc := range []struct {
		// name of the test
		name string
		// path to use in an HTTP GET request
		urlPath string
		// whether to mutate the identifier links in documentation.
		doDocumentationHack bool
		// statusCode we expect to see in the headers.
		wantStatusCode int
		// if non-empty, contents of Location header. For testing redirects.
		wantLocation string
		// if non-nil, call the checker on the HTML root node
		want htmlcheck.Checker
	}{
		{
			name:           "static",
			urlPath:        "/static/",
			wantStatusCode: http.StatusOK,
			want:           in("", text("css"), text("html"), text("img"), text("js")),
		},
		{
			name:           "license policy",
			urlPath:        "/license-policy",
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".Content-header", text("License Disclaimer")),
				in(".Content",
					text("The Go website displays license information"),
					text("this is not legal advice"))),
		},
		{
			// just check that it returns 200
			name:           "favicon",
			urlPath:        "/favicon.ico",
			wantStatusCode: http.StatusOK,
			want:           nil,
		},
		{
			name:           "robots.txt",
			urlPath:        "/robots.txt",
			wantStatusCode: http.StatusOK,
			want:           in("", text("User-agent: *"), text(regexp.QuoteMeta("Disallow: /*?tab=*"))),
		},
		{
			name:           "search",
			urlPath:        fmt.Sprintf("/search?q=%s", sample.PackageName),
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".SearchResults-resultCount", text("2 results")),
				in(".SearchSnippet-header",
					in("a",
						href("/github.com/valid_module_name/foo?tab=overview"),
						text("github.com/valid_module_name/foo")))),
		},
		{
			name:           "package default",
			urlPath:        fmt.Sprintf("/%s?tab=doc", sample.PackagePath),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, unversioned),
				in(".Documentation", text(`This is the documentation HTML`))),
		},
		{
			name:           "package default redirect",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath),
			wantStatusCode: http.StatusFound,
			wantLocation:   "/github.com/valid_module_name/foo?tab=doc",
		},
		{
			name: "package default nonredistributable",
			// For a non-redistributable package, the "latest" route goes to the overview tab.
			urlPath:        "/github.com/non_redistributable/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(pkgNonRedist, unversioned),
		},
		{
			name:           "package@version default",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Documentation", text(`This is the documentation HTML`))),
		},
		{
			name: "package@version default specific version nonredistributable",
			// For a non-redistributable package, the name@version route goes to the overview tab.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(pkgNonRedist, versioned),
		},
		{
			name:           "package@version doc tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, "v0.9.0", pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV090, versioned),
				in(".Documentation", text(`This is the documentation HTML`))),
		},
		{
			name:           "package@version doc with links",
			urlPath:        "/github.com/valid_module_name/foo/directory/hello?tab=doc",
			wantStatusCode: http.StatusOK,
			want: in(".Documentation",
				in("a", href("/pkg/io#Writer"), text("io.Writer"))),
		},
		{
			name:                "package@version doc with hacked up links",
			urlPath:             "/github.com/valid_module_name/foo/directory/hello?tab=doc",
			doDocumentationHack: true,
			wantStatusCode:      http.StatusOK,
			want: in(".Documentation",
				in("a", href("/io?tab=doc#Writer"), text("io.Writer"))),
		},
		{
			name: "package@version doc tab nonredistributable",
			// For a non-redistributable package, the doc tab will not show the doc.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar?tab=doc",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgNonRedist, versioned),
				in(".DetailsContent", text(`not displayed due to license restrictions`))),
		},
		{
			name:           "package@version readme tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/github.com/valid_module_name@v1.0.0",
					ModuleLinkText: pkgV100.ModulePath,
					RepoURL:        "https://github.com/valid_module_name",
					PackageURL:     "https://github.com/valid_module_name/tree/v1.0.0/foo",
					ReadmeContent:  "readme",
					ReadmeSource:   "github.com/valid_module_name@v1.0.0/README.md",
				})),
		},
		{
			name: "package@version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgNonRedist, versioned),
				in(".DetailsContent", text(`not displayed due to license restrictions`))),
		},
		{
			name:           "package@version subdirectories tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Directories",
					in("a",
						href(fmt.Sprintf("/%s@%s/%s/directory/hello", sample.ModulePath, sample.VersionString, pkgSuffix)),
						text("directory/hello")))),
		},
		{
			name:           "package@version versions tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Versions",
					text(`Versions`),
					text(`v1`),
					in("a",
						href("/github.com/valid_module_name@v1.0.0/foo"),
						attr("title", "v1.0.0"),
						text("v1.0.0")))),
		},
		{
			name:           "package@version imports tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in("li.selected", text(`Imports`)),
				in(".Imports-heading", text(`Standard Library Imports`)),
				in(".Imports-list",
					inAt("a", 0, href("/fmt"), text("fmt")),
					inAt("a", 1, href("/path/to/bar"), text("path/to/bar")))),
		},
		{
			name:           "package@version imported by tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".EmptyContent-message", text(`No known importers for this package`))),
		},
		{
			name:           "package@version imported by tab second page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".EmptyContent-message", text(`No known importers for this package`))),
		},
		{
			name:           "package@version licenses tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", "github.com/valid_module_name@v1.0.0/LICENSE")),
		},
		{
			name:           "package@version overview tab, pseudoversion",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, pseudoVersion, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgPseudo, versioned)),
		},
		{
			name:           "package@version overview tab, +incompatible",
			urlPath:        "/github.com/incompatible@v1.0.0+incompatible/dir/inc?tab=overview",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgInc, versioned)),
		},
		{
			name:           "directory subdirectories",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, unversioned),
				// TODO(b/144217401) link should be unversioned.
				pagecheck.SubdirectoriesDetails("/github.com/valid_module_name@v1.0.0/foo/directory/hello", "hello")),
		},
		{
			name:           "directory@version subdirectories",
			urlPath:        "/github.com/valid_module_name@v1.0.0/foo/directory",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, versioned),
				pagecheck.SubdirectoriesDetails("/github.com/valid_module_name@v1.0.0/foo/directory/hello", "hello")),
		},
		{
			name:           "directory@version subdirectories pseudoversion",
			urlPath:        "/github.com/pseudo@" + pseudoVersion + "/dir?tab=subdirectories",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirPseudo, versioned),
				pagecheck.SubdirectoriesDetails("/github.com/pseudo@"+pseudoVersion+"/dir/baz", "baz")),
		},
		{
			name:           "directory subdirectories pseudoversion",
			urlPath:        "/github.com/pseudo/dir?tab=subdirectories",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirPseudo, unversioned),
				// TODO(b/144217401) link should be unversioned.
				pagecheck.SubdirectoriesDetails("/github.com/pseudo@"+pseudoVersion+"/dir/baz", "baz")),
		},
		{
			name:           "directory overview",
			urlPath:        fmt.Sprintf("/%s?tab=overview", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, unversioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/github.com/valid_module_name",
					ModuleLinkText: dir.ModulePath,
					RepoURL:        "https://github.com/valid_module_name",
					ReadmeContent:  "readme",
					ReadmeSource:   "github.com/valid_module_name@v1.0.0/README.md",
				})),
		},
		{
			name:           "directory licenses",
			urlPath:        fmt.Sprintf("/%s?tab=licenses", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, unversioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", "github.com/valid_module_name@v1.0.0/LICENSE")),
		},
		{
			name:           "stdlib directory default",
			urlPath:        "/cmd",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, unversioned),
				pagecheck.SubdirectoriesDetails("", "")),
		},
		{
			name:           "stdlib directory subdirectories",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=subdirectories"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, versioned),
				pagecheck.SubdirectoriesDetails("", "")),
		},
		{
			name:           "stdlib directory overview",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=overview"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/std@go1.13",
					ModuleLinkText: "Standard Library",
					ReadmeContent:  "readme",
					RepoURL:        "https://github.com/valid_module_name", // wrong, but hard to change
					ReadmeSource:   "go.googlesource.com/go/+/refs/tags/go1.13/README.md",
				})),
		},
		{
			name:           "stdlib directory licenses",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=licenses"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, versioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", "go.googlesource.com/go/+/refs/tags/go1.13/LICENSE")),
		},
		{
			name:           "module default",
			urlPath:        fmt.Sprintf("/mod/%s", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			want: in("",
				pagecheck.ModuleHeader(mod, unversioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/" + sample.ModulePath,
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://github.com/valid_module_name",
					ReadmeSource:   "github.com/valid_module_name@v1.0.0/README.md",
				})),
		},
		{
			name:           "module overview",
			urlPath:        fmt.Sprintf("/mod/%s?tab=overview", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			want: in("",
				pagecheck.ModuleHeader(mod, unversioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/" + sample.ModulePath,
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://github.com/valid_module_name",
					ReadmeSource:   "github.com/valid_module_name@v1.0.0/README.md",
				})),
		},
		{
			name:           "module overview pseudoversion latest",
			urlPath:        "/mod/github.com/pseudo?tab=overview",
			wantStatusCode: http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			want: in("",
				pagecheck.ModuleHeader(mod2, unversioned),
				in(".Overview-module a",
					href("/mod/github.com/pseudo"),
					text("^github.com/pseudo$")),
				in(".Overview-readmeContent", text(`readme`))),
		},

		// // TODO(b/139498072): add a second module, so we can verify that we get the latest version.
		{
			name:           "module packages tab latest version",
			urlPath:        fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Fall back to the latest version.
			want: in("",
				pagecheck.ModuleHeader(mod, unversioned),
				in(".Directories", text(`This is a package synopsis`))),
		},
		{
			name:           "module@version overview tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     fmt.Sprintf("/mod/%s@%s", sample.ModulePath, sample.VersionString),
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://github.com/valid_module_name",
					ReadmeSource:   "github.com/valid_module_name@v1.0.0/README.md",
				})),
		},
		{
			name:           "module@version overview tab, pseudoversion",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, pseudoVersion),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(modPseudo, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     fmt.Sprintf("/mod/%s@%s", sample.ModulePath, pseudoVersion),
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://github.com/valid_module_name",
					ReadmeSource:   "github.com/valid_module_name@" + pseudoVersion + "/README.md",
				})),
		},
		{
			name:           "module@version packages tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				in(".Directories", text(`This is a package synopsis`))),
		},
		{
			name:           "module@version versions tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				in("li.selected", text(`Versions`)),
				in("div.Versions", text("v1")),
				in("li.Versions-item",
					in("a",
						href("/mod/github.com/valid_module_name@v1.0.0"),
						attr("title", "v1.0.0"),
						text("v1.0.0")))),
		},
		{
			name:           "module@version licenses tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", "github.com/valid_module_name@v1.0.0/LICENSE")),
		},
		{
			name:           "cmd go package page",
			urlPath:        "/cmd/go?tab=doc",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(cmdGo, unversioned),
		},
		{
			name:           "cmd go package page at version",
			urlPath:        "/cmd/go@go1.13?tab=doc",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(cmdGo, versioned),
		},
		{
			name:           "standard library module page",
			urlPath:        "/std",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.ModuleHeader(std, unversioned),
		},
		{
			name:           "standard library module page at version",
			urlPath:        "/std@go1.13",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.ModuleHeader(std, versioned),
		},
		{
			name:           "latest version for the standard library",
			urlPath:        "/latest-version/std",
			wantStatusCode: http.StatusOK,
			want:           in("", text(`"go1.13"`)),
		},
		{
			name:           "latest version for module",
			urlPath:        "/latest-version/" + sample.ModulePath,
			wantStatusCode: http.StatusOK,
			want:           in("", text(`"v1.0.0"`)),
		},
		{
			name:           "latest version for package",
			urlPath:        "/latest-version/github.com/valid_module_name?pkg=github.com/valid_module_name/foo/directory/hello",
			wantStatusCode: http.StatusOK,
			want:           in("", text(`"v1.0.0"`)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) { // remove initial '/' for name
			defer func(orig bool) { doDocumentationHack = orig }(doDocumentationHack)
			doDocumentationHack = tc.doDocumentationHack
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", tc.urlPath, nil))
			res := w.Result()
			if res.StatusCode != tc.wantStatusCode {
				t.Errorf("GET %q = %d, want %d", tc.urlPath, res.StatusCode, tc.wantStatusCode)
			}
			if tc.wantLocation != "" {
				if got := res.Header.Get("Location"); got != tc.wantLocation {
					t.Errorf("Location: got %q, want %q", got, tc.wantLocation)
				}
			}
			doc, err := html.Parse(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()

			if tc.want != nil {
				if err := tc.want(doc); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestServerErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	sampleVersion := sample.Version()
	if err := testDB.InsertVersion(ctx, sampleVersion); err != nil {
		t.Fatal(err)
	}

	s, err := NewServer(testDB, nil, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/invalid-page", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status code: got = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func mustRequest(urlPath string, t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, "http://localhost"+urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPackageTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/host.com/module/suffix", t), shortTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
	}
	for _, test := range tests {
		if got := packageTTL(test.r); got != test.want {
			t.Errorf("packageTTL(%v) = %v, want %v", test.r, got, test.want)
		}
	}
}

func TestModuleTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/mod/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/mod/host.com/module/suffix", t), shortTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
	}
	for _, test := range tests {
		if got := moduleTTL(test.r); got != test.want {
			t.Errorf("packageTTL(%v) = %v, want %v", test.r, got, test.want)
		}
	}
}

func TestTagRoute(t *testing.T) {
	mustRequest := func(url string) *http.Request {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	tests := []struct {
		route string
		req   *http.Request
		want  string
	}{
		{"/pkg", mustRequest("http://localhost/pkg/foo?tab=versions"), "pkg-versions"},
		{"/", mustRequest("http://localhost/foo?tab=imports"), "imports"},
	}
	for _, test := range tests {
		if got := TagRoute(test.route, test.req); got != test.want {
			t.Errorf("TagRoute(%q, %v) = %q, want %q", test.route, test.req, got, test.want)
		}
	}
}
