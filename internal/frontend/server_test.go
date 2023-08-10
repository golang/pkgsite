// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/alicebob/miniredis/v2 pulls in
// github.com/yuin/gopher-lua which uses a non
// build-tag-guarded use of the syscall package.
//go:build !plan9

package frontend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/safehtml/template"
	"github.com/jba/templatecheck"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/frontend/page"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
	"golang.org/x/pkgsite/internal/testing/pagecheck"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/static"
)

func TestHTMLInjection(t *testing.T) {
	_, handler, _ := newTestServer(t, nil, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

const pseudoVersion = "v0.0.0-20140414041502-123456789012"

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
}

// Units with this prefix will be marked as excluded.
const excludedModulePath = "github.com/module/excluded"

const moduleLinksReadme = `
# Heading
Stuff

## Links
- [title1](http://url1)
- [title2](javascript://pwned)
`

const packageLinksReadme = `
# Links
- [pkg title](http://url2)
`

var testModules = []testModule{
	{
		// An ordinary module, with three versions.
		path:            sample.ModulePath,
		redistributable: true,
		versions:        []string{"v1.0.0", "v0.9.0", pseudoVersion},
		packages: []testPackage{
			{
				suffix:         "foo",
				readmeContents: sample.ReadmeContents,
				readmeFilePath: sample.ReadmeFilePath,
			},
			{
				suffix: "foo/directory/hello",
			},
		},
	},
	{
		// A module with a greater major version available.
		path:            "github.com/v2major/module_name",
		redistributable: true,
		versions:        []string{"v1.0.0"},
		packages: []testPackage{
			{
				suffix:         "bar",
				readmeContents: sample.ReadmeContents,
				readmeFilePath: sample.ReadmeFilePath,
			},
			{
				suffix: "bar/directory/hello",
			},
			{
				suffix:         "buz",
				readmeContents: sample.ReadmeContents,
				readmeFilePath: sample.ReadmeFilePath,
			},
			{
				suffix: "buz/directory/hello",
			},
		},
	},
	{
		// A v2 of the previous module, with one version.
		path:            "github.com/v2major/module_name/v2",
		redistributable: true,
		versions:        []string{"v2.0.0"},
		packages: []testPackage{
			{
				suffix:         "bar",
				readmeContents: sample.ReadmeContents,
				readmeFilePath: sample.ReadmeFilePath,
			},
			{
				suffix: "bar/directory/hello",
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
	{
		// A module with links.
		path:            "github.com/links/mod",
		redistributable: true,
		versions:        []string{"v1.0.0"},
		packages: []testPackage{
			{
				suffix:         "",
				readmeFilePath: sample.ReadmeFilePath, // required
				readmeContents: moduleLinksReadme,
			},
			{
				// This package has no readme, just links in its godoc. So the
				// UnitMeta (right sidebar) links will be from the godoc and
				// module readme.
				suffix: "no_readme",
			},
			{
				// This package has a readme as well as godoc links, so the
				// UnitMeta links will be from all three places.
				suffix:         "has_readme",
				readmeFilePath: "has_readme/README.md", // required
				readmeContents: packageLinksReadme,
			},
		},
	},
	{
		path:            excludedModulePath,
		redistributable: true,
		versions:        []string{sample.VersionString},
		packages: []testPackage{
			{
				name:   "pkg",
				suffix: "pkg",
			},
		},
	},
	{
		path:            "cloud.google.com/go",
		redistributable: true,
		versions:        []string{"v0.69.0"},
		packages: []testPackage{
			{
				name:   "pubsublite",
				suffix: "pubsublite",
			},
		},
	},
	{
		path:            "cloud.google.com/go/pubsublite",
		redistributable: true,
		versions:        []string{"v0.4.0"},
		packages: []testPackage{
			{
				name:   "pubsublite",
				suffix: "",
			},
		},
	},
	// A module with a package that is not in the module's latest version. Since
	// our testModule struct can only describe modules where all packages are at
	// all versions, we need two of them.
	{
		path:            "golang.org/x/tools",
		redistributable: true,
		versions:        []string{"v1.1.0"},
		packages: []testPackage{
			{name: "blog", suffix: "blog"},
			{name: "vet", suffix: "cmd/vet"},
		},
	},
	{
		path:            "golang.org/x/tools",
		redistributable: true,
		versions:        []string{"v1.2.0"},
		packages: []testPackage{
			{name: "blog", suffix: "blog"},
		},
	},
	// A module with a package that has documentation for two build contexts.
	{
		path:            "a.com/two",
		redistributable: true,
		versions:        []string{"v1.2.3"},
		packages: []testPackage{
			{
				name:   "pkg",
				suffix: "pkg",
				docs: []*internal.Documentation{
					sample.Documentation("linux", "amd64", `package p; var L int`),
					sample.Documentation("windows", "amd64", `package p; var W int`),
				},
			},
		},
	},
}

var (
	in      = htmlcheck.In
	notIn   = htmlcheck.NotIn
	hasText = htmlcheck.HasText
	attr    = htmlcheck.HasAttr

	// href checks for an exact match in an href attribute.
	href = func(val string) htmlcheck.Checker {
		return attr("href", "^"+regexp.QuoteMeta(val)+"$")
	}
)

var notAtLatestPkg = &pagecheck.Page{
	ModulePath:             "golang.org/x/tools",
	Suffix:                 "cmd/vet",
	Title:                  "vet",
	ModuleURL:              "github.com/golang/tools",
	Version:                "v1.1.0",
	FormattedVersion:       "v1.1.0",
	LicenseType:            "MIT",
	LicenseFilePath:        "LICENSE",
	MissingInMinor:         true,
	IsLatestMajor:          true,
	UnitURLFormat:          "/golang.org/x/tools/cmd/vet%s",
	LatestLink:             "/golang.org/x/tools/cmd/vet",
	LatestMajorVersionLink: "/golang.org/x/tools",
}

// serverTestCases are the test cases valid for any experiment. For experiments
// that modify any part of the behaviour covered by the test cases in
// serverTestCase(), a new test generator should be created and added to
// TestServer().
func serverTestCases() []serverTestCase {
	const (
		versioned   = true
		unversioned = false
		isPackage   = true
		isDirectory = false
	)

	pkgV100 := &pagecheck.Page{
		Title:                  "foo",
		ModulePath:             sample.ModulePath,
		Version:                sample.VersionString,
		FormattedVersion:       sample.VersionString,
		Suffix:                 sample.Suffix,
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/" + sample.ModulePath + "/" + sample.Suffix,
		LatestMajorVersionLink: "/" + sample.ModulePath + "/" + sample.Suffix,
		LicenseType:            sample.LicenseType,
		LicenseFilePath:        sample.LicenseFilePath,
		UnitURLFormat:          "/" + sample.ModulePath + "%s/" + sample.Suffix,
		ModuleURL:              "/" + sample.ModulePath,
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	v2pkgV100 := &pagecheck.Page{
		Title:                  "bar",
		ModulePath:             "github.com/v2major/module_name",
		Version:                "v1.0.0",
		FormattedVersion:       "v1.0.0",
		Suffix:                 "bar",
		IsLatestMinor:          true,
		IsLatestMajor:          false,
		LatestLink:             "/github.com/v2major/module_name/bar",
		LatestMajorVersion:     "v2",
		LatestMajorVersionLink: "/github.com/v2major/module_name/v2/bar",
		LicenseType:            sample.LicenseType,
		LicenseFilePath:        sample.LicenseFilePath,
		UnitURLFormat:          "/github.com/v2major/module_name%s/bar",
		ModuleURL:              "/github.com/v2major/module_name",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	v2pkgV1Buz := *v2pkgV100
	v2pkgV1Buz.Title = "buz"
	v2pkgV1Buz.Suffix = "buz"
	v2pkgV1Buz.IsLatestMajor = false
	v2pkgV1Buz.IsLatestMinor = true
	v2pkgV1Buz.LatestLink = "/github.com/v2major/module_name/buz"
	v2pkgV1Buz.LatestMajorVersionLink = "/github.com/v2major/module_name/v2"
	v2pkgV1Buz.UnitURLFormat = "/github.com/v2major/module_name%s/buz"

	v2pkgV200 := &pagecheck.Page{
		Title:                  "bar",
		ModulePath:             "github.com/v2major/module_name/v2",
		Version:                "v2.0.0",
		FormattedVersion:       "v2.0.0",
		Suffix:                 "bar",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/github.com/v2major/module_name/v2/bar",
		LatestMajorVersion:     "v2",
		LatestMajorVersionLink: "/github.com/v2major/module_name/v2/bar",
		LicenseType:            sample.LicenseType,
		LicenseFilePath:        sample.LicenseFilePath,
		UnitURLFormat:          "/github.com/v2major/module_name/v2%s/bar",
		ModuleURL:              "/github.com/v2major/module_name/v2",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	p9 := *pkgV100
	p9.Version = "v0.9.0"
	p9.FormattedVersion = "v0.9.0"
	p9.IsLatestMinor = false
	p9.IsLatestMajor = true
	pkgV090 := &p9

	pp := *pkgV100
	pp.Version = pseudoVersion
	pp.FormattedVersion = "v0.0.0-...-1234567"
	pp.IsLatestMinor = false
	pkgPseudo := &pp

	pkgInc := &pagecheck.Page{
		Title:                  "inc",
		ModulePath:             "github.com/incompatible",
		Version:                "v1.0.0+incompatible",
		FormattedVersion:       "v1.0.0+incompatible",
		Suffix:                 "dir/inc",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/github.com/incompatible/dir/inc",
		LatestMajorVersionLink: "/github.com/incompatible/dir/inc",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		UnitURLFormat:          "/github.com/incompatible%s/dir/inc",
		ModuleURL:              "/github.com/incompatible",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	pkgNonRedist := &pagecheck.Page{
		Title:                  "bar",
		ModulePath:             "github.com/non_redistributable",
		Version:                "v1.0.0",
		FormattedVersion:       "v1.0.0",
		Suffix:                 "bar",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/github.com/non_redistributable/bar",
		LatestMajorVersionLink: "/github.com/non_redistributable/bar",
		LicenseType:            "",
		UnitURLFormat:          "/github.com/non_redistributable%s/bar",
		ModuleURL:              "/github.com/non_redistributable",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	dir := &pagecheck.Page{
		Title:                  "directory/",
		ModulePath:             sample.ModulePath,
		Version:                "v1.0.0",
		FormattedVersion:       "v1.0.0",
		Suffix:                 "foo/directory",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		ModuleURL:              "/" + sample.ModulePath,
		UnitURLFormat:          "/" + sample.ModulePath + "%s/foo/directory",
		LatestMajorVersionLink: "/github.com/valid/module_name/foo/directory",
		LatestLink:             "/github.com/valid/module_name/foo/directory",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	mod := &pagecheck.Page{
		ModulePath:             sample.ModulePath,
		Title:                  "module_name",
		ModuleURL:              "/" + sample.ModulePath,
		Version:                "v1.0.0",
		FormattedVersion:       "v1.0.0",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/" + sample.ModulePath + "@v1.0.0",
		LatestMajorVersionLink: "/" + sample.ModulePath,
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}
	mp := *mod
	mp.Version = pseudoVersion
	mp.FormattedVersion = "v0.0.0-...-1234567"
	mp.IsLatestMinor = false
	mp.IsLatestMajor = true

	dirPseudo := &pagecheck.Page{
		ModulePath:             "github.com/pseudo",
		Title:                  "dir/",
		ModuleURL:              "/github.com/pseudo",
		LatestLink:             "/github.com/pseudo/dir",
		LatestMajorVersionLink: "/github.com/pseudo/dir",
		Suffix:                 "dir",
		Version:                pseudoVersion,
		FormattedVersion:       mp.FormattedVersion,
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		UnitURLFormat:          "/github.com/pseudo%s/dir",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	dirCmd := &pagecheck.Page{
		Title:                  "cmd",
		ModulePath:             "std",
		Version:                "go1.13",
		FormattedVersion:       "go1.13",
		Suffix:                 "cmd",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMajor:          true,
		IsLatestMinor:          true,
		ModuleURL:              "/std",
		LatestLink:             "/cmd",
		UnitURLFormat:          "/cmd%s",
		LatestMajorVersionLink: "/cmd",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	netHttp := &pagecheck.Page{
		Title:                  "http",
		ModulePath:             "http",
		Version:                "go1.13",
		FormattedVersion:       "go1.13",
		LicenseType:            sample.LicenseType,
		LicenseFilePath:        sample.LicenseFilePath,
		ModuleURL:              "/net/http",
		UnitURLFormat:          "/net/http%s",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		LatestLink:             "/net/http",
		LatestMajorVersionLink: "/net/http",
		CommitTime:             absoluteTime(sample.NowTruncated()),
	}

	cloudMod := &pagecheck.Page{
		ModulePath:             "cloud.google.com/go",
		ModuleURL:              "cloud.google.com/go",
		Title:                  "go",
		Suffix:                 "go",
		Version:                "v0.69.0",
		FormattedVersion:       "v0.69.0",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		UnitURLFormat:          "/cloud.google.com/go%s",
		LatestLink:             "/cloud.google.com/go",
		LatestMajorVersionLink: "/cloud.google.com/go",
	}

	pubsubliteDir := &pagecheck.Page{
		ModulePath:             "cloud.google.com/go",
		ModuleURL:              "cloud.google.com/go",
		Title:                  "pubsublite",
		Suffix:                 "pubsublite",
		Version:                "v0.69.0",
		FormattedVersion:       "v0.69.0",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          false,
		IsLatestMajor:          true,
		UnitURLFormat:          "/cloud.google.com/go%s/pubsublite",
		LatestLink:             "/cloud.google.com/go/pubsublite",
		LatestMajorVersionLink: "/cloud.google.com/go/pubsublite",
	}

	pubsubliteMod := &pagecheck.Page{
		ModulePath:             "cloud.google.com/go/pubsublite",
		Title:                  "pubsublite",
		ModuleURL:              "cloud.google.com/go/pubsublite",
		Version:                "v0.4.0",
		FormattedVersion:       "v0.4.0",
		LicenseType:            "MIT",
		LicenseFilePath:        "LICENSE",
		IsLatestMinor:          true,
		IsLatestMajor:          true,
		UnitURLFormat:          "/cloud.google.com/go/pubsublite%s",
		LatestLink:             "/cloud.google.com/go/pubsublite",
		LatestMajorVersionLink: "/cloud.google.com/go/pubsublite",
	}

	return []serverTestCase{
		{
			name:           "C",
			urlPath:        "/C",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/cmd/cgo",
		},
		{
			name:           "github golang std",
			urlPath:        "/github.com/golang/go",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/std",
		},
		{
			name:           "github golang std src",
			urlPath:        "/github.com/golang/go/src",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/std",
		},
		{
			name:           "github golang time",
			urlPath:        "/github.com/golang/go/time",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/time",
		},
		{
			name:           "github golang time src",
			urlPath:        "/github.com/golang/go/src/time",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/time",
		},
		{
			name:           "github golang x tools repo 404 instead of redirect",
			urlPath:        "/github.com/golang/tools",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "github golang x tools go packages 404 instead of redirect",
			urlPath:        "/github.com/golang/tools/go/packages",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "static",
			urlPath:        "/static/",
			wantStatusCode: http.StatusOK,
			want:           in("", hasText("doc"), hasText("frontend"), hasText("shared"), hasText("worker")),
		},
		{
			name:           "license policy",
			urlPath:        "/license-policy",
			wantStatusCode: http.StatusOK,
			want: in("",
				in("h1", hasText("License Disclaimer")),
				in(".go-Content",
					hasText("The Go website displays license information"),
					hasText("this is not legal advice"))),
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
			want:           in("", hasText("User-agent: *"), hasText(regexp.QuoteMeta("Disallow: /search?*"))),
		},
		{
			name:           "search large offset",
			urlPath:        "/search?q=github.com&page=1002",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "bad version",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v1-2", sample.Suffix),
			wantStatusCode: http.StatusBadRequest,
			want: in("",
				in("h3.Error-message", hasText("v1-2 is not a valid semantic version.")),
				in("p.Error-message a", href(`/search?q=github.com%2fvalid%2fmodule_name%2ffoo`))),
		},
		{
			name:           "unknown version",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v99.99.0", sample.Suffix),
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Fetch-message.js-fetchMessage", hasText(sample.ModulePath+"/foo@v99.99.0"))),
		},
		{

			name:           "path not found",
			urlPath:        "/example.com/unknown",
			wantStatusCode: http.StatusNotFound,
			want: in("",
				in("h3.Fetch-message.js-fetchMessage", hasText("example.com/unknown"))),
		},
		{
			name:           "bad request, invalid github module path",
			urlPath:        "/github.com/foo",
			wantStatusCode: http.StatusBadRequest,
			want:           in("h3.Error-message", hasText("is not a valid import path")),
		},
		{
			name:           "excluded",
			urlPath:        "/" + excludedModulePath + "/pkg",
			wantStatusCode: http.StatusNotFound,
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
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/http",
		},
		{
			name:           "stdlib shortcut with args and trailing slash",
			urlPath:        "/http@go1.13/",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/http@go1.13",
		},
		{
			name:           "package page with trailiing slash",
			urlPath:        "/github.com/my/module/",
			wantStatusCode: http.StatusMovedPermanently,
			wantLocation:   "/github.com/my/module",
		},
		{
			name:           "package default",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgV100, unversioned, isPackage),
				notIn(".UnitBuildContext-titleContext")),
		},
		{
			name:           "package default redirect",
			urlPath:        fmt.Sprintf("/%s?tab=doc", sample.PackagePath),
			wantStatusCode: http.StatusFound,
			wantLocation:   "/" + sample.ModulePath + "/foo",
		},
		{
			name:           "package default nonredistributable",
			urlPath:        "/github.com/non_redistributable/bar",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgNonRedist, unversioned, isPackage),
				in(".UnitDetails-content", hasText(`not displayed due to license restrictions`)),
			),
		},
		{
			name:           "v2 package at v1",
			urlPath:        fmt.Sprintf("/%s@%s/%s", v2pkgV100.ModulePath, v2pkgV100.Version, v2pkgV100.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(v2pkgV100, versioned, isPackage),
				pagecheck.UnitReadme(),
				pagecheck.UnitDoc(),
				pagecheck.UnitDirectories(fmt.Sprintf("/%s@%s/%s/directory/hello", v2pkgV100.ModulePath, v2pkgV100.Version, v2pkgV100.Suffix), "hello"),
				pagecheck.CanonicalURLPath("/github.com/v2major/module_name@v1.0.0/bar")),
		},
		{
			name:           "v2 module with v1 package that does not exist in v2",
			urlPath:        fmt.Sprintf("/%s@%s/%s", v2pkgV1Buz.ModulePath, v2pkgV1Buz.Version, v2pkgV1Buz.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(&v2pkgV1Buz, versioned, isPackage),
				pagecheck.UnitReadme(),
				pagecheck.UnitDoc(),
				pagecheck.UnitDirectories(fmt.Sprintf("/%s@%s/%s/directory/hello", v2pkgV1Buz.ModulePath, v2pkgV1Buz.Version, v2pkgV1Buz.Suffix), "hello"),
				pagecheck.CanonicalURLPath("/github.com/v2major/module_name@v1.0.0/buz")),
		},
		{
			name:           "v2 package at v2",
			urlPath:        fmt.Sprintf("/%s@%s/%s", v2pkgV200.ModulePath, v2pkgV200.Version, v2pkgV200.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(v2pkgV200, versioned, isPackage),
				pagecheck.UnitReadme(),
				pagecheck.UnitDoc(),
				pagecheck.UnitDirectories(fmt.Sprintf("/%s@%s/%s/directory/hello", v2pkgV200.ModulePath, v2pkgV200.Version, v2pkgV200.Suffix), "hello"),
				pagecheck.CanonicalURLPath("/github.com/v2major/module_name/v2@v2.0.0/bar")),
		},
		{
			name:           "package at version default",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgV100, versioned, isPackage),
				pagecheck.UnitReadme(),
				pagecheck.UnitDoc(),
				pagecheck.UnitDirectories(fmt.Sprintf("/%s@%s/%s/directory/hello", sample.ModulePath, sample.VersionString, sample.Suffix), "hello"),
				pagecheck.CanonicalURLPath("/github.com/valid/module_name@v1.0.0/foo")),
		},
		{
			name:           "package at version default specific version nonredistributable",
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgNonRedist, versioned, isPackage),
				in(".UnitDetails-content", hasText(`not displayed due to license restrictions`)),
			),
		},
		{
			name:           "package at version",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, "v0.9.0", sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgV090, versioned, isPackage),
				pagecheck.UnitReadme(),
				pagecheck.UnitDoc(),
				pagecheck.CanonicalURLPath("/github.com/valid/module_name@v0.9.0/foo")),
		},
		{
			name:           "package at version nonredistributable",
			urlPath:        "/github.com/non_redistributable@v1.0.0/bar",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgNonRedist, versioned, isPackage),
				in(".UnitDetails-content", hasText(`not displayed due to license restrictions`))),
		},
		{
			name:           "package at version versions page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in(".Versions",
				hasText(`v1`),
				in("a",
					href("/"+sample.ModulePath+"@v1.0.0/foo"),
					hasText("v1.0.0"))),
		},
		{
			name:           "package at version imports page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".Imports-heading", hasText(`Standard library imports`)),
				in(".Imports-list",
					in("li:nth-child(1) a", href("/fmt"), hasText("fmt")),
					in("li:nth-child(2) a", href("/path/to/bar"), hasText("path/to/bar")))),
		},
		{
			name:           "package at version imported by tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want:           in(`[data-test-id="gopher-message"]`, hasText(`No known importers for this package`)),
		},
		{
			name:           "package at version imported by tab second page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want:           in(`[data-test-id="gopher-message"]`, hasText(`No known importers for this package`)),
		},
		{
			name:           "package at version licenses tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", sample.ModulePath+"@v1.0.0/LICENSE")),
		},
		{
			name:           "package at version, pseudoversion",
			urlPath:        fmt.Sprintf("/%s@%s/%s", sample.ModulePath, pseudoVersion, sample.Suffix),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgPseudo, versioned, isPackage)),
		},
		{
			name:           "stdlib no shortcut (net/http)",
			urlPath:        "/net/http",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.UnitHeader(netHttp, unversioned, isPackage),
		},
		{
			name:           "stdlib no shortcut (net/http) versioned",
			urlPath:        "/net/http@go1.13",
			wantStatusCode: http.StatusOK,
			want:           pagecheck.UnitHeader(netHttp, versioned, isPackage),
		},
		{
			name:           "package at version, +incompatible",
			urlPath:        "/github.com/incompatible@v1.0.0+incompatible/dir/inc",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pkgInc, versioned, isPackage)),
		},
		{
			name:           "directory subdirectories",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dir, unversioned, isDirectory),
				// TODO(golang/go#39630) link should be unversioned.
				pagecheck.UnitDirectories("/"+sample.ModulePath+"@v1.0.0/foo/directory/hello", "hello")),
		},
		{
			name:           "directory@version subdirectories",
			urlPath:        "/" + sample.ModulePath + "@v1.0.0/foo/directory",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dir, versioned, isDirectory),
				pagecheck.UnitDirectories("/"+sample.ModulePath+"@v1.0.0/foo/directory/hello", "hello")),
		},
		{
			name:           "directory@version subdirectories pseudoversion",
			urlPath:        "/github.com/pseudo@" + pseudoVersion + "/dir",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dirPseudo, versioned, isDirectory),
				pagecheck.UnitDirectories("/github.com/pseudo@"+pseudoVersion+"/dir/baz", "baz")),
		},
		{
			name:           "directory subdirectories pseudoversion",
			urlPath:        "/github.com/pseudo/dir",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dirPseudo, unversioned, isDirectory),
				// TODO(golang/go#39630) link should be unversioned.
				pagecheck.UnitDirectories("/github.com/pseudo@"+pseudoVersion+"/dir/baz", "baz")),
		},
		{
			name:           "directory",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dir, unversioned, isDirectory),
				pagecheck.CanonicalURLPath("/github.com/valid/module_name@v1.0.0/foo")),
		},
		{
			name:           "directory licenses",
			urlPath:        fmt.Sprintf("/%s?tab=licenses", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", sample.ModulePath+"@v1.0.0/LICENSE")),
		},
		{
			name:           "stdlib directory default",
			urlPath:        "/cmd",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dirCmd, unversioned, isDirectory),
				pagecheck.UnitDirectories("", "")),
		},
		{
			name:           "stdlib directory versioned",
			urlPath:        "/cmd@go1.13",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(dirCmd, versioned, isDirectory),
				pagecheck.UnitDirectories("", "")),
		},
		{
			name:           "stdlib directory licenses",
			urlPath:        "/cmd@go1.13?tab=licenses",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.LicenseDetails("MIT", "Lorem Ipsum", "go.googlesource.com/go/+/refs/tags/go1.13/LICENSE")),
		},
		{
			name:           "pubsublite unversioned",
			urlPath:        "/cloud.google.com/go/pubsublite",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pubsubliteMod, unversioned, isPackage)),
		},
		{
			name:           "pubsublite module",
			urlPath:        "/cloud.google.com/go/pubsublite@v0.4.0",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pubsubliteMod, versioned, isPackage)),
		},
		{
			name:           "pubsublite directory",
			urlPath:        "/cloud.google.com/go@v0.69.0/pubsublite",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(pubsubliteDir, versioned, isDirectory)),
		},
		{
			name:           "cloud.google.com/go module",
			urlPath:        "/cloud.google.com/go",
			wantStatusCode: http.StatusOK,
			want: in("",
				pagecheck.UnitHeader(cloudMod, unversioned, isDirectory)),
		},
		{
			name:           "two docs default",
			urlPath:        "/a.com/two/pkg",
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".Documentation-variables", hasText("var L")),
				in(".UnitBuildContext-titleContext", hasText("linux/amd64"))),
		},
		{
			name:           "two docs linux",
			urlPath:        "/a.com/two/pkg?GOOS=linux",
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".Documentation-variables", hasText("var L")),
				in(".UnitBuildContext-titleContext", hasText("linux/amd64"))),
		},
		{
			name:           "two docs windows",
			urlPath:        "/a.com/two/pkg?GOOS=windows",
			wantStatusCode: http.StatusOK,
			want: in("",
				in(".Documentation-variables", hasText("var W")),
				in(".UnitBuildContext-titleContext", hasText("windows/amd64"))),
		},
		{
			name:           "two docs no match",
			urlPath:        "/a.com/two/pkg?GOOS=dragonfly",
			wantStatusCode: http.StatusOK,
			want: in("",
				notIn(".Documentation-variables"),
				notIn(".UnitBuildContext-titleContext")),
		},
	}
}

func checkLink(title, url string) htmlcheck.Checker {
	// The first div under .UnitMeta is "Repository", the second is "Links",
	// and each subsequent div contains a <a> tag with a custom link.
	return in(fmt.Sprintf(`[data-test-id="meta-link-%s"]`, title), href(url), hasText(title))
}

var linksTestCases = []serverTestCase{
	{
		name:           "module links",
		urlPath:        "/github.com/links/mod",
		wantStatusCode: http.StatusOK,
		want: in("",
			// Module readme links.
			checkLink("title1", "http://url1"),
			checkLink("title2", "about:invalid#zGoSafez"),
		),
	},
	{
		name:           "no_readme package links",
		urlPath:        "/github.com/links/mod/no_readme",
		wantStatusCode: http.StatusOK,
		want: in("",
			// Package doc links are first.
			checkLink("pkg.go.dev", "https://pkg.go.dev"),
			// Then module readmes.
			checkLink("title1", "http://url1"),
			checkLink("title2", "about:invalid#zGoSafez"),
		),
	},
	{
		name:           "has_readme package links",
		urlPath:        "/github.com/links/mod/has_readme",
		wantStatusCode: http.StatusOK,
		want: in("",
			// Package readme links are first.
			checkLink("pkg title", "http://url2"),
			// Package doc links are second.
			checkLink("pkg.go.dev", "https://pkg.go.dev"),
			// Module readme links are third.
			checkLink("title1", "http://url1"),
			checkLink("title2", "about:invalid#zGoSafez"),
		),
	},
	{
		name: "not at latest",
		// A package which is at its own latest minor version but not at the
		// latest minor version of its module.
		urlPath:        "/golang.org/x/tools/cmd/vet",
		wantStatusCode: http.StatusOK,
		want:           in("", pagecheck.UnitHeader(notAtLatestPkg, false, true)),
	},
}

var searchGroupingTestCases = []serverTestCase{
	{
		name:           "search",
		urlPath:        fmt.Sprintf("/search?q=%s", sample.PackageName),
		wantStatusCode: http.StatusOK,
		want: in("",
			in(`[data-test-id="pagination"]`, hasText("See  search help")),
			in(`[data-test-id="snippet-title"]`,
				href("/"+sample.ModulePath+"/foo"),
				hasText("foo"))),
	},
	{
		name:           "pageless search",
		urlPath:        fmt.Sprintf("/search?q=%s", sample.PackageName),
		wantStatusCode: http.StatusOK,
		want: in("",
			in(`[data-test-id="pagination"]`, hasText("See  search help")),
			notIn(".Pagination-navInner")),
	},
	{
		name:           "search large limit",
		urlPath:        fmt.Sprintf("/search?q=%s&limit=101", sample.PackageName),
		wantStatusCode: http.StatusBadRequest,
	},
	{
		name:           "no results",
		urlPath:        "/search?q=xyzzy",
		wantStatusCode: http.StatusOK,
		want:           notIn(".Pagination-nav"),
	},
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
	}{
		{
			name: "no experiments",
			testCasesFunc: func() []serverTestCase {
				cases := serverTestCases()
				cases = append(cases, linksTestCases...)
				return append(cases, searchGroupingTestCases...)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testServer(t, test.testCasesFunc())
		})
	}
}

func testServer(t *testing.T, testCases []serverTestCase) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	insertTestModules(ctx, t, testModules)
	if err := testDB.InsertExcludedPrefix(ctx, excludedModulePath, "testuser", "testreason"); err != nil {
		t.Fatal(err)
	}
	_, _, handler, _ := newTestServerWithFetch(t, nil, nil)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) { // remove initial '/' for name
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.urlPath, nil))
			res := w.Result()
			if res.StatusCode != test.wantStatusCode {
				t.Fatalf("GET %q = %d, want %d", test.urlPath, res.StatusCode, test.wantStatusCode)
			}
			if test.wantLocation != "" {
				if got := res.Header.Get("Location"); got != test.wantLocation {
					t.Errorf("Location: got %q, want %q", got, test.wantLocation)
				}
			}
			doc, err := html.Parse(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()

			if test.want != nil {
				if err := test.want(doc); err != nil {
					if testing.Verbose() {
						html.Render(os.Stdout, doc)
					}
					t.Error(err)
				}
			}
		})
	}
}

func findCookie(name string, cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func checkBody(body io.ReadCloser, c htmlcheck.Checker) error {
	doc, err := html.Parse(body)
	if err != nil {
		return err
	}
	_ = body.Close()
	return c(doc)
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
		{
			func() *http.Request {
				r := mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t)
				r.Header.Set("user-agent",
					"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)")
				return r
			}(),
			tinyTTL,
		},
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
		{"/search", mustRequest("http://localhost/search?q=net&m=vuln"), "search-vuln"},
		{"/search", mustRequest("http://localhost/search?q=net&m=package"), "search-package"},
		{"/search", mustRequest("http://localhost/search?q=net&m=symbol"), "search-symbol"},
		{"/search", mustRequest("http://localhost/search?q=net"), "search-package"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := TagRoute(test.route, test.req); got != test.want {
				t.Errorf("TagRoute(%q, %v) = %q, want %q", test.route, test.req, got, test.want)
			}
		})
	}
}

func TestCheckTemplates(t *testing.T) {
	// Perform additional checks on parsed templates.
	staticFS := template.TrustedFSFromEmbed(static.FS)
	templates, err := parsePageTemplates(staticFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		subs    []string
		typeval any
	}{
		{"badge", nil, badgePage{}},
		// error.tmpl omitted because relies on an associated "message" template
		// that's parsed on demand; see renderErrorPage above.
		{"fetch", nil, page.ErrorPage{}},
		{"homepage", nil, homepage{}},
		{"license-policy", nil, licensePolicyPage{}},
		{"search", nil, SearchPage{}},
		{"search-help", nil, page.BasePage{}},
		{"unit/main", nil, UnitPage{}},
		{
			"unit/main",
			[]string{"unit-outline", "unit-readme", "unit-doc", "unit-files", "unit-directories"},
			MainDetails{},
		},
		{"unit/importedby", nil, UnitPage{}},
		{"unit/importedby", []string{"importedby"}, ImportedByDetails{}},
		{"unit/imports", nil, UnitPage{}},
		{"unit/imports", []string{"imports"}, ImportsDetails{}},
		{"unit/licenses", nil, UnitPage{}},
		{"unit/licenses", []string{"licenses"}, LicensesDetails{}},
		{"unit/versions", nil, UnitPage{}},
		{"unit/versions", []string{"versions"}, VersionsDetails{}},
		{"vuln", nil, page.BasePage{}},
		{"vuln/list", nil, VulnListPage{}},
		{"vuln/entry", nil, VulnEntryPage{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			tm := templates[c.name]
			if tm == nil {
				t.Fatalf("no template %q", c.name)
			}
			if c.subs == nil {
				if err := templatecheck.CheckSafe(tm, c.typeval); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, n := range c.subs {
					s := tm.Lookup(n)
					if s == nil {
						t.Fatalf("no sub-template %q of %q", n, c.name)
					}
					if err := templatecheck.CheckSafe(s, c.typeval); err != nil {
						t.Fatalf("%s: %v", n, err)
					}
				}
			}
		})
	}
}

func TestStripScheme(t *testing.T) {
	for _, test := range []struct {
		url, want string
	}{
		{"http://github.com", "github.com"},
		{"https://github.com/path/to/something", "github.com/path/to/something"},
		{"example.com", "example.com"},
		{"chrome-extension://abcd", "abcd"},
		{"nonwellformed.com/path?://query=1", "query=1"},
	} {
		if got := stripScheme(test.url); got != test.want {
			t.Errorf("%q: got %q, want %q", test.url, got, test.want)
		}
	}
}

func TestInstallFS(t *testing.T) {
	s, handler, teardown := newTestServer(t, nil, nil)
	defer teardown()
	s.InstallFS("/dir", os.DirFS("."))
	// Request this file.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/files/dir/server_test.go", nil))
	if w.Code != http.StatusOK {
		t.Errorf("got status code = %d, want %d", w.Code, http.StatusOK)
	}
	if want := "TestInstallFS"; !strings.Contains(w.Body.String(), want) {
		t.Errorf("body does not contain %q", want)
	}
}
