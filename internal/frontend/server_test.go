// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
	"golang.org/x/pkgsite/internal/testing/pagecheck"
	"golang.org/x/pkgsite/internal/testing/sample"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

func TestHTMLInjection(t *testing.T) {
	_, handler, _ := newTestServer(t, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

const pseudoVersion = "v0.0.0-20140414041502-123456789012"

type testModule struct {
	path            string
	redistributable bool
	versions        []string
	packages        []testPackage
}

type testPackage struct {
	name   string
	suffix string
	doc    string
}

type serverTestCase struct {
	// name of the test
	name string
	// path to use in an HTTP GET request
	urlPath string
	// statusCode we expect to see in the headers.
	wantStatusCode int
	// if non-empty, contents of Location header. For testing redirects.
	wantLocation string
	// if non-nil, call the checker on the HTML root node
	want htmlcheck.Checker
	// list of experiments that must be enabled for this test to run
	requiredExperiments *experiment.Set
}

var testModules = []testModule{
	{
		// An ordinary module, with three versions.
		path:            sample.ModulePath,
		redistributable: true,
		versions:        []string{"v1.0.0", "v0.9.0", pseudoVersion},
		packages: []testPackage{
			{
				suffix: "foo",
				doc:    sample.DocumentationHTML.String(),
			},
			{
				suffix: "foo/directory/hello",
				doc:    `<a href="/pkg/io#Writer">io.Writer</a>`,
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
				suffix: "bar",
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
				suffix: "dir/baz",
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
				suffix: "dir/inc",
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
				name:   "main",
				suffix: "cmd/go",
			},
			{
				name:   "http",
				suffix: "net/http",
			},
		},
	},
}

func insertTestModules(ctx context.Context, t *testing.T, mods []testModule) {
	for _, mod := range mods {
		var ps []*internal.LegacyPackage
		for _, pkg := range mod.packages {
			p := sample.LegacyPackage(mod.path, pkg.suffix)
			p.DocumentationHTML = safehtml.HTML{}
			if pkg.name != "" {
				p.Name = pkg.name
			}
			if pkg.doc != "" {
				p.DocumentationHTML = testconversions.MakeHTMLForTest(pkg.doc)
			}
			if !mod.redistributable {
				p.Licenses = nil
				p.IsRedistributable = false
			}
			p.V1Path = sample.V1Path
			ps = append(ps, p)
		}
		for _, ver := range mod.versions {
			m := sample.Module(mod.path, ver)
			m.SourceInfo = source.NewGitHubInfo(sample.RepositoryURL, "", ver)
			m.IsRedistributable = mod.redistributable
			if !m.IsRedistributable {
				m.Licenses = nil
			}
			for _, p := range ps {
				sample.AddPackage(m, p)
			}
			for _, u := range m.Units {
				if !mod.redistributable {
					u.IsRedistributable = false
					u.Licenses = nil
				}
			}
			if err := testDB.InsertModule(ctx, m); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// serverTestCases are the test cases valid for any experiment. For experiments
// that modify any part of the behaviour covered by the test cases in
// serverTestCase(), a new test generator should be created and added to
// TestServer().
func serverTestCases() []serverTestCase {
	const (
		versioned   = true
		unversioned = false
	)

	var (
		in   = htmlcheck.In
		text = htmlcheck.HasText
		attr = htmlcheck.HasAttr

		// href checks for an exact match in an href attribute.
		href = func(val string) htmlcheck.Checker {
			return attr("href", "^"+regexp.QuoteMeta(val)+"$")
		}
	)

	pkgV100 := &pagecheck.Page{
		Title:            "Package foo",
		ModulePath:       sample.ModulePath,
		Version:          "v1.0.0",
		Suffix:           "foo",
		IsLatest:         true,
		LatestLink:       "/" + sample.ModulePath + "@v1.0.0/foo",
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		PackageURLFormat: "/" + sample.ModulePath + "%s/foo",
		ModuleURL:        "/mod/" + sample.ModulePath,
	}
	p9 := *pkgV100
	p9.Version = "v0.9.0"
	p9.IsLatest = false
	pkgV090 := &p9

	pp := *pkgV100
	pp.Version = pseudoVersion
	pp.FormattedVersion = "v0.0.0-...-1234567"
	pp.IsLatest = false
	pkgPseudo := &pp

	pkgInc := &pagecheck.Page{
		Title:            "Package inc",
		ModulePath:       "github.com/incompatible",
		Version:          "v1.0.0+incompatible",
		Suffix:           "dir/inc",
		IsLatest:         true,
		LatestLink:       "/github.com/incompatible@v1.0.0+incompatible/dir/inc",
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		PackageURLFormat: "/github.com/incompatible%s/dir/inc",
		ModuleURL:        "/mod/github.com/incompatible",
	}

	pkgNonRedist := &pagecheck.Page{
		Title:            "Package bar",
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
		Title:            "Command go",
		ModulePath:       "std",
		Suffix:           "cmd/go",
		Version:          "go1.13",
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		IsLatest:         true,
		LatestLink:       "/cmd/go@go1.13",
		PackageURLFormat: "/cmd/go%s",
		ModuleURL:        "/std",
	}
	mod := &pagecheck.Page{
		ModulePath:      sample.ModulePath,
		Title:           "Module " + sample.ModulePath,
		ModuleURL:       "/mod/" + sample.ModulePath,
		Version:         "v1.0.0",
		LicenseType:     "MIT",
		LicenseFilePath: "LICENSE",
		IsLatest:        true,
		LatestLink:      "/mod/" + sample.ModulePath + "@v1.0.0",
	}
	mp := *mod
	mp.Version = pseudoVersion
	mp.FormattedVersion = "v0.0.0-...-1234567"
	mp.IsLatest = false
	modPseudo := &mp

	mod2 := &pagecheck.Page{
		ModulePath:       "github.com/pseudo",
		Title:            "Module github.com/pseudo",
		ModuleURL:        "/mod/github.com/pseudo",
		LatestLink:       "/mod/github.com/pseudo@" + pseudoVersion,
		Version:          pseudoVersion,
		FormattedVersion: mp.FormattedVersion,
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		IsLatest:         true,
	}
	dirPseudo := &pagecheck.Page{
		ModulePath:       "github.com/pseudo",
		Title:            "Directory github.com/pseudo/dir",
		ModuleURL:        "/mod/github.com/pseudo",
		LatestLink:       "/mod/github.com/pseudo@" + pseudoVersion + "/dir",
		Suffix:           "dir",
		Version:          pseudoVersion,
		FormattedVersion: mp.FormattedVersion,
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		IsLatest:         true,
		PackageURLFormat: "/github.com/pseudo%s/dir",
	}

	std := &pagecheck.Page{
		Title:           "Standard library",
		ModulePath:      "std",
		Version:         "go1.13",
		LicenseType:     "MIT",
		LicenseFilePath: "LICENSE",
		ModuleURL:       "/std",
		IsLatest:        true,
		LatestLink:      "/std@go1.13",
	}

	netHttp := &pagecheck.Page{
		Title:           "Package http",
		ModulePath:      "http",
		Version:         "go1.13",
		LicenseType:     "MIT",
		LicenseFilePath: "LICENSE",
		ModuleURL:       "/net/http",
		IsLatest:        true,
		LatestLink:      "/net/http@go1.13",
	}

	dir := &pagecheck.Page{
		Title:            "Directory " + sample.ModulePath + "/foo/directory",
		ModulePath:       sample.ModulePath,
		Version:          "v1.0.0",
		Suffix:           "foo/directory",
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		ModuleURL:        "/mod/" + sample.ModulePath,
		PackageURLFormat: "/" + sample.ModulePath + "%s/foo/directory",
	}

	dirCmd := &pagecheck.Page{
		Title:            "Directory cmd",
		ModulePath:       "std",
		Version:          "go1.13",
		Suffix:           "cmd",
		LicenseType:      "MIT",
		LicenseFilePath:  "LICENSE",
		ModuleURL:        "/std",
		PackageURLFormat: "/cmd%s",
	}

	testCases := []serverTestCase{
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
			want:           in("", text("User-agent: *"), text(regexp.QuoteMeta("Disallow: /search?*"))),
		},
		{
			name:           "search",
			urlPath:        fmt.Sprintf("/search?q=%s", sample.PackageName),
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".SearchResults-resultCount", text("2 results")),
				in(".SearchSnippet-header",
					in("a",
						href("/"+sample.ModulePath+"/foo"),
						text(sample.ModulePath+"/foo")))),
		},
		{
			name:           "package default",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, unversioned),
				in("div.DetailsHeader-breadcrumb",
					in("a",
						href("/"+sample.ModulePath+"@v1.0.0"),
						text(sample.ModulePath))),
				in(".Documentation", text(`This is the documentation HTML`))),
		},
		{
			name:           "package default redirect",
			urlPath:        fmt.Sprintf("/%s?tab=doc", sample.PackagePath),
			wantStatusCode: http.StatusFound,
			wantLocation:   "/" + sample.ModulePath + "/foo",
		},
		{
			name: "package default nonredistributable",
			// For a non-redistributable package, the "latest" route goes to the overview tab.
			urlPath:        "/github.com/non_redistributable/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(pkgNonRedist, unversioned),
		},
		{
			name:           "package at version default",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Documentation", text(`This is the documentation HTML`)),
				in(".js-canonicalURLPath", attr("data-canonical-url-path", "/github.com/valid/module_name@v1.0.0/foo"))),
		},
		{
			name: "package at version default specific version nonredistributable",
			// For a non-redistributable package, the name@version route goes to the overview tab.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(pkgNonRedist, versioned),
		},
		{
			name:           "package at version doc tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v0.9.0", sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV090, versioned),
				in(".Documentation", text(`This is the documentation HTML`))),
		},
		{
			name: "package at version doc tab nonredistributable",
			// For a non-redistributable package, the doc tab will not show the doc.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgNonRedist, versioned),
				in(".DetailsContent", text(`not displayed due to license restrictions`))),
		},
		{
			name:           "package at version doc tab, no doc",
			urlPath:        "/github.com/pseudo@" + pseudoVersion + "/dir/baz",
			wantStatusCode: http.StatusOK,
			want:           in("", text("No documentation available")),
		},
		{
			name:           "package at version readme tab redistributable",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/" + sample.ModulePath + "@v1.0.0",
					ModuleLinkText: pkgV100.ModulePath,
					RepoURL:        "https://" + sample.ModulePath,
					PackageURL:     "https://" + sample.ModulePath + "/tree/v1.0.0/foo",
					ReadmeContent:  "readme",
					ReadmeSource:   sample.ModulePath + "@v1.0.0/README.md",
				})),
		},
		{
			name: "package at version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar?tab=overview",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgNonRedist, versioned),
				in(".DetailsContent", text(`not displayed due to license restrictions`))),
		},
		{
			name:           "package at version subdirectories tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Directories",
					in("a",
						href(fmt.Sprintf("/%s@%s/%s/directory/hello", sample.ModulePath, sample.VersionString, sample.Suffix)),
						text("directory/hello")))),
		},
		{
			name:           "package at version versions tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".Versions",
					text(`v1`),
					in("a",
						href("/"+sample.ModulePath+"@v1.0.0/foo"),
						text("v1.0.0")))),
		},
		{
			name:           "package at version imports tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in("[role='tab'][aria-selected='true']", text(`Imports`)),
				in(".Imports-heading", text(`Standard Library Imports`)),
				in(".Imports-list",
					in("li:nth-child(1) a", href("/fmt"), text("fmt")),
					in("li:nth-child(2) a", href("/path/to/bar"), text("path/to/bar")))),
		},
		{
			name:           "package at version imported by tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".EmptyContent-message", text(`No known importers for this package`))),
		},
		{
			name:           "package at version imported by tab second page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				in(".EmptyContent-message", text(`No known importers for this package`))),
		},
		{
			name:           "package at version licenses tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgV100, versioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", sample.ModulePath+"@v1.0.0/LICENSE")),
		},
		{
			name:           "package at version overview tab, pseudoversion",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, pseudoVersion, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.PackageHeader(pkgPseudo, versioned)),
		},
		{
			name:           "package at version overview tab, +incompatible",
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
				in("div.DetailsHeader-breadcrumb",
					in("a:nth-of-type(1)",
						href("/"+sample.ModulePath+"@v1.0.0"),
						text(sample.ModulePath)),
					in("a:nth-of-type(2)",
						href("/"+sample.ModulePath+"/foo@v1.0.0"),
						text("foo"))),
				// TODO(golang/go#39630) link should be unversioned.
				pagecheck.SubdirectoriesDetails("/"+sample.ModulePath+"@v1.0.0/foo/directory/hello", "hello")),
		},
		{
			name:           "directory@version subdirectories",
			urlPath:        "/" + sample.ModulePath + "@v1.0.0/foo/directory",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, versioned),
				pagecheck.SubdirectoriesDetails("/"+sample.ModulePath+"@v1.0.0/foo/directory/hello", "hello")),
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
				// TODO(golang/go#39630) link should be unversioned.
				pagecheck.SubdirectoriesDetails("/github.com/pseudo@"+pseudoVersion+"/dir/baz", "baz")),
		},
		{
			name:           "directory overview",
			urlPath:        fmt.Sprintf("/%s?tab=overview", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, unversioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/mod/" + sample.ModulePath,
					ModuleLinkText: dir.ModulePath,
					RepoURL:        "https://" + sample.ModulePath,
					ReadmeContent:  "readme",
					ReadmeSource:   sample.ModulePath + "@v1.0.0/README.md",
				}),
				in(".js-canonicalURLPath", attr("data-canonical-url-path", "/github.com/valid/module_name@v1.0.0/foo"))),
		},
		{
			name:           "directory licenses",
			urlPath:        fmt.Sprintf("/%s?tab=licenses", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dir, unversioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", sample.ModulePath+"@v1.0.0/LICENSE")),
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
			urlPath:        "/cmd@go1.13?tab=subdirectories",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, versioned),
				pagecheck.SubdirectoriesDetails("", "")),
		},
		{
			name:           "stdlib directory overview",
			urlPath:        "/cmd@go1.13?tab=overview",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.DirectoryHeader(dirCmd, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     "/std@go1.13",
					ModuleLinkText: "Standard Library",
					ReadmeContent:  "readme",
					RepoURL:        "https://" + sample.ModulePath, // wrong, but hard to change
					ReadmeSource:   "go.googlesource.com/go/+/refs/tags/go1.13/README.md",
				})),
		},
		{
			name:           "stdlib directory licenses",
			urlPath:        "/cmd@go1.13?tab=licenses",
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
					RepoURL:        "https://" + sample.ModulePath,
					ReadmeSource:   sample.ModulePath + "@v1.0.0/README.md",
				}),
				in(".js-canonicalURLPath", attr("data-canonical-url-path", "/mod/github.com/valid/module_name@v1.0.0"))),
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
					RepoURL:        "https://" + sample.ModulePath,
					ReadmeSource:   sample.ModulePath + "@v1.0.0/README.md",
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
		{
			name:           "module packages tab latest version",
			urlPath:        fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Fall back to the latest version.
			want: in("",
				pagecheck.ModuleHeader(mod, unversioned),
				in(".Directories", text(`This is a package synopsis`)),
				in("div.DetailsHeader-version", text("v1.0.0"))),
		},
		{
			name:           "module at version overview tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     fmt.Sprintf("/mod/%s@%s", sample.ModulePath, sample.VersionString),
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://" + sample.ModulePath,
					ReadmeSource:   sample.ModulePath + "@v1.0.0/README.md",
				})),
		},
		{
			name:           "module at version overview tab, pseudoversion",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, pseudoVersion),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(modPseudo, versioned),
				pagecheck.OverviewDetails(&pagecheck.Overview{
					ModuleLink:     fmt.Sprintf("/mod/%s@%s", sample.ModulePath, pseudoVersion),
					ModuleLinkText: sample.ModulePath,
					ReadmeContent:  "readme",
					RepoURL:        "https://" + sample.ModulePath,
					ReadmeSource:   sample.ModulePath + "@" + pseudoVersion + "/README.md",
				})),
		},
		{
			name:           "module at version packages tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				in(".Directories", text(`This is a package synopsis`))),
		},
		{
			name:           "module at version versions tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				in("[role='tab'][aria-selected='true']", text(`Versions`)),
				in("div.Versions", text("v1")),
				in("li.Versions-item",
					in("a",
						href("/mod/"+sample.ModulePath+"@v1.0.0"),
						text("v1.0.0")))),
		},
		{
			name:           "module at version licenses tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.ModuleHeader(mod, versioned),
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", sample.ModulePath+"@v1.0.0/LICENSE")),
		},
		{
			name:           "cmd go package page",
			urlPath:        "/cmd/go",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.PackageHeader(cmdGo, unversioned),
		},
		{
			name:           "cmd go package page at version",
			urlPath:        "/cmd/go@go1.13",
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
			name:           "bad version",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v1-2", sample.Suffix),
			wantStatusCode: http.StatusBadRequest,
			want: in("",
				in("h3.Error-message", text("v1-2 is not a valid semantic version.")),
				in("p.Error-message a", href(`/search?q=github.com%2fvalid%2fmodule_name%2ffoo`))),
		},
		{
			name:           "stdlib no shortcut (net/http)",
			urlPath:        "/net/http",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.ModuleHeader(netHttp, unversioned),
		},
		{
			name:           "unknown version, no experiments",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v99.99.0", sample.Suffix),
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Error-message", text("Package "+sample.ModulePath+"/foo@v99.99.0 is not available.")),
				in("p.Error-message a", href("/"+sample.ModulePath+"/foo?tab=versions"))),
		},
		{

			name:           "path not found",
			urlPath:        "/example.com/unknown",
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Fetch-message.js-fetchMessage", text("example.com/unknown"))),
		},
		{

			name:           "module page for path that is a package but not a module",
			urlPath:        "/mod/" + sample.ModulePath + "/foo",
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Fetch-message.js-fetchMessage", text(sample.ModulePath+"/foo"))),
		},
		{
			name:           "stdlib shortcut (net/http)",
			urlPath:        "/http",
			wantStatusCode: http.StatusFound,
			wantLocation:   "/net/http",
		},
		{
			name:           "stdlib shortcut (net/http) strip args",
			urlPath:        "/http@go1.13",
			wantStatusCode: http.StatusFound,
			wantLocation:   "/net/http",
		},
		{
			name:           "stdlib shortcut with trailing slash",
			urlPath:        "/http/",
			wantStatusCode: http.StatusFound,
			wantLocation:   "/net/http",
		},
		{
			name:           "stdlib shortcut with args and trailing slash",
			urlPath:        "/http@go1.13/",
			wantStatusCode: http.StatusFound,
			wantLocation:   "/net/http",
		},
	}

	return testCases
}

// frontendFetchTestCases() returns test cases that are valid if
// internal.ExperimentFrontendFetch is active.
func frontendFetchTestCases() []serverTestCase {
	var (
		in   = htmlcheck.In
		text = htmlcheck.HasText
	)

	return []serverTestCase{
		{
			name:           "unknown version, frontend experiment",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v99.99.0", sample.Suffix),
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Fetch-message.js-fetchMessage", text(sample.ModulePath+"/foo@v99.99.0"))),
			requiredExperiments: experiment.NewSet(internal.ExperimentFrontendFetch),
		},
		{
			name:                "bad request, invalid github module path, frontend experiment",
			urlPath:             "/github.com/foo",
			wantStatusCode:      http.StatusBadRequest,
			requiredExperiments: experiment.NewSet(internal.ExperimentFrontendFetch),
		},
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
func TestServer(t *testing.T) {
	for _, test := range []struct {
		name          string
		testCasesFunc func() []serverTestCase
		experiments   []string
	}{
		{
			name:          "no experiments",
			testCasesFunc: serverTestCases,
		},
		{
			name:          "use directories",
			testCasesFunc: serverTestCases,
			experiments:   []string{internal.ExperimentUseUnits},
		},
		{
			name:          "frontend fetch",
			testCasesFunc: frontendFetchTestCases,
			experiments:   []string{internal.ExperimentFrontendFetch},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testServer(t, test.testCasesFunc(), test.experiments...)
		})
	}
}

func testServer(t *testing.T, testCases []serverTestCase, experimentNames ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	// Experiments need to be set in the context, for DB work, and as a
	// middleware, for request handling.
	ctx = experiment.NewContext(ctx, experimentNames...)
	insertTestModules(ctx, t, testModules)
	_, handler, _ := newTestServer(t, nil, experimentNames...)

	experimentsSet := experiment.NewSet(experimentNames...)

	for _, tc := range testCases {
		if !isSubset(tc.requiredExperiments, experimentsSet) {
			continue
		}

		t.Run(tc.name, func(t *testing.T) { // remove initial '/' for name
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

			checkIDs(t, doc)

			if tc.want != nil {
				if err := tc.want(doc); err != nil {
					if testing.Verbose() {
						html.Render(os.Stdout, doc)
					}
					t.Error(err)
				}
			}
		})
	}
}

func isSubset(subset, set *experiment.Set) bool {
	for _, e := range subset.Active() {
		if !set.IsActive(e) {
			return false
		}
	}

	return true
}

var badIDRegexp = regexp.MustCompile(`^[a-zA-Z0-9_.]*$`)

func checkIDs(t *testing.T, n *html.Node) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "id") && badIDRegexp.MatchString(a.Val) {
			t.Errorf("HTML id %q must not be a valid Go identifier or dotted expression (add a hyphen)", a.Val)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		checkIDs(t, c)
	}
}

func TestServerErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	sampleModule := sample.DefaultModule()
	if err := testDB.InsertModule(ctx, sampleModule); err != nil {
		t.Fatal(err)
	}
	_, handler, _ := newTestServer(t, nil)

	for _, test := range []struct {
		name, path string
		wantCode   int
	}{
		{"not found", "/invalid-page", http.StatusNotFound},
		{"bad request", "/gocloud.dev/@latest/blob", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
			if w.Code != test.wantCode {
				t.Errorf("%q: got status code = %d, want %d", test.path, w.Code, test.wantCode)
			}
		})
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

func TestDetailsTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/host.com/module/suffix", t), shortTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/mod/host.com/module/suffix", t), shortTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
	}
	for _, test := range tests {
		if got := detailsTTL(test.r); got != test.want {
			t.Errorf("detailsTTL(%v) = %v, want %v", test.r, got, test.want)
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
		t.Run(test.want, func(t *testing.T) {
			if got := TagRoute(test.route, test.req); got != test.want {
				t.Errorf("TagRoute(%q, %v) = %q, want %q", test.route, test.req, got, test.want)
			}
		})
	}
}

func newTestServer(t *testing.T, proxyModules []*proxy.Module, experimentNames ...string) (*Server, http.Handler, func()) {
	t.Helper()
	proxyClient, teardown := proxy.SetupTestClient(t, proxyModules)
	sourceClient := source.NewClient(sourceTimeout)
	ctx := context.Background()

	q := queue.NewInMemory(ctx, 1, experimentNames,
		func(ctx context.Context, mpath, version string) (int, error) {
			return FetchAndUpdateState(ctx, mpath, version, proxyClient, sourceClient, testDB)
		})

	s, err := NewServer(ServerConfig{
		DataSourceGetter:     func(context.Context) internal.DataSource { return testDB },
		Queue:                q,
		TaskIDChangeInterval: 10 * time.Minute,
		StaticPath:           template.TrustedSourceFromConstant("../../content/static"),
		ThirdPartyPath:       "../../third_party",
		AppVersionLabel:      "",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil, nil)

	var exps []*internal.Experiment
	for _, n := range experimentNames {
		exps = append(exps, &internal.Experiment{Name: n, Rollout: 100})
	}
	esrc := internal.NewLocalExperimentSource(exps)
	exp, err := middleware.NewExperimenter(ctx, time.Hour, func(context.Context) internal.ExperimentSource { return esrc })
	if err != nil {
		t.Fatal(err)
	}
	mw := middleware.Chain(
		middleware.LatestVersions(s.GetLatestMinorVersion, s.GetLatestMajorVersion),
		middleware.Experiment(exp))
	return s, mw(mux), func() {
		teardown()
		postgres.ResetTestDB(testDB, t)
	}
}
