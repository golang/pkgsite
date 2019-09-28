// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-replayers/httpreplay"
)

var record = flag.Bool("record", false, "record interactions with other systems, for replay")

func TestModuleInfo(t *testing.T) {
	client, done := newReplayClient(t, *record)
	defer done()

	for _, test := range []struct {
		desc                                     string
		modulePath, version, file                string
		wantRepo, wantModule, wantFile, wantLine string
	}{
		{
			"standard library",
			"std", "v1.12.0", "bytes/buffer.go",

			"https://github.com/golang/go",
			"https://github.com/golang/go/tree/go1.12/src",
			"https://github.com/golang/go/blob/go1.12/src/bytes/buffer.go",
			"https://github.com/golang/go/blob/go1.12/src/bytes/buffer.go#L1",
		},
		{
			"old standard library",
			"std", "v1.3.0", "bytes/buffer.go",

			"https://github.com/golang/go",
			"https://github.com/golang/go/tree/go1.3/src/pkg",
			"https://github.com/golang/go/blob/go1.3/src/pkg/bytes/buffer.go",
			"https://github.com/golang/go/blob/go1.3/src/pkg/bytes/buffer.go#L1",
		},
		{
			"github module at repo root",
			"github.com/pkg/errors", "v0.8.1", "errors.go",

			"https://github.com/pkg/errors",
			"https://github.com/pkg/errors/tree/v0.8.1",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go#L1",
		},
		{
			"github module not at repo root",
			"github.com/hashicorp/consul/sdk", "v0.2.0", "freeport/freeport.go",

			"https://github.com/hashicorp/consul",
			"https://github.com/hashicorp/consul/tree/sdk/v0.2.0/sdk",
			"https://github.com/hashicorp/consul/blob/sdk/v0.2.0/sdk/freeport/freeport.go",
			"https://github.com/hashicorp/consul/blob/sdk/v0.2.0/sdk/freeport/freeport.go#L1",
		},
		{
			"bitbucket",
			"bitbucket.org/plazzaro/kami", "v1.2.1", "defaults.go",

			"https://bitbucket.org/plazzaro/kami",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1/defaults.go",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1/defaults.go#lines-1",
		},
		{
			"incompatible",
			"github.com/airbrake/gobrake", "v3.5.1+incompatible", "gobrake.go",

			"https://github.com/airbrake/gobrake",
			"https://github.com/airbrake/gobrake/tree/v3.5.1",
			"https://github.com/airbrake/gobrake/blob/v3.5.1/gobrake.go",
			"https://github.com/airbrake/gobrake/blob/v3.5.1/gobrake.go#L1",
		},
		{
			"x/tools",
			"golang.org/x/tools", "v0.0.0-20190927191325-030b2cf1153e", "README.md",

			"https://github.com/golang/tools",
			"https://github.com/golang/tools/tree/030b2cf1153e",
			"https://github.com/golang/tools/blob/030b2cf1153e/README.md",
			"https://github.com/golang/tools/blob/030b2cf1153e/README.md#L1",
		},
		{
			"x/tools/gopls",
			"golang.org/x/tools/gopls", "v0.1.4", "main.go",

			"https://github.com/golang/tools",
			"https://github.com/golang/tools/tree/gopls/v0.1.4/gopls",
			"https://github.com/golang/tools/blob/gopls/v0.1.4/gopls/main.go",
			"https://github.com/golang/tools/blob/gopls/v0.1.4/gopls/main.go#L1",
		},
		{
			"googlesource.com",
			"go.googlesource.com/image.git", "v0.0.0-20190910094157-69e4b8554b2a", "math/fixed/fixed.go",

			"https://go.googlesource.com/image",
			"https://go.googlesource.com/image/+/69e4b8554b2a",
			"https://go.googlesource.com/image/+/69e4b8554b2a/math/fixed/fixed.go",
			"https://go.googlesource.com/image/+/69e4b8554b2a/math/fixed/fixed.go#1",
		},
		{
			"vanity for github",
			"cloud.google.com/go/spanner", "v1.0.0", "doc.go",

			"https://github.com/googleapis/google-cloud-go",
			"https://github.com/googleapis/google-cloud-go/tree/spanner/v1.0.0/spanner",
			"https://github.com/googleapis/google-cloud-go/blob/spanner/v1.0.0/spanner/doc.go",
			"https://github.com/googleapis/google-cloud-go/blob/spanner/v1.0.0/spanner/doc.go#L1",
		},
		{
			"vanity for bitbucket",
			"badc0de.net/pkg/glagolitic", "v0.0.0-20180930175637-92f736eb02d6", "doc.go",

			"https://bitbucket.org/ivucica/go-glagolitic",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6/doc.go",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6/doc.go#lines-1",
		},
		{
			"vanity for googlesource.com",
			"cuelang.org/go", "v0.0.9", "cuego/doc.go",

			"https://cue.googlesource.com/cue",
			"https://cue.googlesource.com/cue/+/v0.0.9",
			"https://cue.googlesource.com/cue/+/v0.0.9/cuego/doc.go",
			"https://cue.googlesource.com/cue/+/v0.0.9/cuego/doc.go#1",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			check := func(msg, got, want string) {
				if got != want {
					t.Fatalf("%s:\ngot  %s\nwant %s", msg, got, want)
				}
				res, err := client.Head(got)
				if err != nil {
					t.Fatalf("%s: %v", got, err)
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					t.Fatalf("%s: %v", got, res.Status)
				}
			}

			info, err := moduleInfo(context.Background(), client, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}

			check("repo", info.RepoURL, test.wantRepo)
			check("module", info.ModuleURL(), test.wantModule)
			check("file", info.FileURL(test.file), test.wantFile)
			check("line", info.LineURL(test.file, 1), test.wantLine)
		})
	}
}

func newReplayClient(t *testing.T, record bool) (*http.Client, func()) {
	replayFilePath := filepath.Join("testdata", t.Name()+".replay")
	if record {
		httpreplay.DebugHeaders()
		t.Logf("Recording into %s", replayFilePath)
		if err := os.MkdirAll(filepath.Dir(replayFilePath), 0755); err != nil {
			t.Fatal(err)
		}
		rec, err := httpreplay.NewRecorder(replayFilePath, nil)
		if err != nil {
			t.Fatal(err)
		}
		return rec.Client(), func() {
			if err := rec.Close(); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		rep, err := httpreplay.NewReplayer(replayFilePath)
		if err != nil {
			t.Fatal(err)
		}
		return rep.Client(), func() { _ = rep.Close() }
	}
}

func TestMatchStatic(t *testing.T) {
	// Most of matchStatic is tested in TestModuleInfo, above. This
	// covers a few other cases.
	for _, test := range []struct {
		in                string
		wantRepo, wantDir string
	}{
		{"github.com/a/b", "github.com/a/b", ""},
		{"bitbucket.org/a/b", "bitbucket.org/a/b", ""},
		{"github.com/a/b/c/d", "github.com/a/b", "c/d"},
		{"bitbucket.org/a/b/c/d", "bitbucket.org/a/b", "c/d"},
		{"foo.googlesource.com/a/b/c", "foo.googlesource.com/a/b/c", ""},
		{"foo.googlesource.com/a/b/c.git", "foo.googlesource.com/a/b/c", ""},
		{"foo.googlesource.com/a/b/c.git/d", "foo.googlesource.com/a/b/c", "d"},
		{"mercurial.com/repo.hg", "mercurial.com/repo", ""},
		{"mercurial.com/repo.hg/dir", "mercurial.com/repo", "dir"},
	} {
		t.Run(test.in, func(t *testing.T) {
			gotRepo, gotDir, _, err := matchStatic(test.in)
			if err != nil {
				t.Fatal(err)
			}
			if gotRepo != test.wantRepo || gotDir != test.wantDir {
				t.Errorf("got %q, %q; want %q, %q", gotRepo, gotDir, test.wantRepo, test.wantDir)
			}
		})
	}
}

// This test adapted from gddo/gosrc/gosrc_test.go:TestGetDynamic.
func TestModuleImportDynamic(t *testing.T) {
	// For this test, fake the HTTP requests so we can cover cases that may not appear in the wild.
	client := &http.Client{Transport: testTransport(testWeb)}
	// The version doesn't figure into the interesting work and we test versions to commits
	// elsewhere, so use the same version throughout.
	const version = "v1.2.3"
	for _, test := range []struct {
		modulePath string
		want       *Info // if nil, then want error
	}{
		{
			"alice.org/pkg",
			&Info{
				RepoURL:   "https://github.com/alice/pkg",
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/sub",
			&Info{
				RepoURL:   "https://github.com/alice/pkg",
				moduleDir: "sub",
				commit:    "sub/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/http",
			&Info{
				RepoURL:   "https://github.com/alice/pkg",
				moduleDir: "http",
				commit:    "http/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/source",
			// Has a go-source tag, but we can't use the templates.
			&Info{
				RepoURL:   "http://alice.org/pkg",
				moduleDir: "source",
				commit:    "source/v1.2.3",
				// empty templates
			},
		},

		{
			"alice.org/pkg/ignore",
			// Stop at the first go-source.
			&Info{
				RepoURL:   "http://alice.org/pkg",
				moduleDir: "ignore",
				commit:    "ignore/v1.2.3",
				// empty templates
			},
		},
		{"alice.org/pkg/multiple", nil},
		{"alice.org/pkg/notfound", nil},
		{
			"bob.com/pkg",
			&Info{
				// The go-import tag's repo root ends in ".git", but according to the spec
				// there should not be a .vcs suffix, so we include the ".git" in the repo URL.
				RepoURL:   "https://vcs.net/bob/pkg.git",
				moduleDir: "",
				commit:    "v1.2.3",
				// empty templates
			},
		},
		{
			"bob.com/pkg/sub",
			&Info{
				RepoURL:   "https://vcs.net/bob/pkg.git",
				moduleDir: "sub",
				commit:    "sub/v1.2.3",
				// empty templates
			},
		},
		{
			"azul3d.org/examples/abs",
			// The go-source tag has a template that is handled incorrectly by godoc; but we
			// ignore the templates.
			&Info{
				RepoURL:   "https://github.com/azul3d/examples",
				moduleDir: "abs",
				commit:    "abs/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"myitcv.io/blah2",
			// Ignore the "mod" vcs type.
			&Info{
				RepoURL:   "https://github.com/myitcv/x",
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/default",
			&Info{
				RepoURL:   "https://github.com/alice/pkg",
				moduleDir: "default",
				commit:    "default/v1.2.3",
				templates: githubURLTemplates,
			},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			got, err := moduleInfoDynamic(context.Background(), client, test.modulePath, version)
			if err != nil {
				if test.want == nil {
					return
				}
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, test.want, cmp.AllowUnexported(Info{}, urlTemplates{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type testTransport map[string]string

func (t testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	statusCode := http.StatusOK
	req.URL.RawQuery = ""
	body, ok := t[req.URL.String()]
	if !ok {
		statusCode = http.StatusNotFound
	}
	resp := &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
	}
	return resp, nil
}

func TestCommitFromVersion(t *testing.T) {
	for _, test := range []struct {
		version, dir string
		want         string
	}{
		{
			"v1.2.3", "",
			"v1.2.3",
		},
		{
			"v1.2.3", "foo",
			"foo/v1.2.3",
		},
		{
			"v6.1.1-0.20190615154606-3a9541ec9974", "",
			"3a9541ec9974",
		},
		{
			"v6.1.1-0.20190615154606-3a9541ec9974", "foo",
			"3a9541ec9974",
		},
	} {
		t.Run(fmt.Sprintf("%s,%s", test.version, test.dir), func(t *testing.T) {
			if got := commitFromVersion(test.version, test.dir); got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
			// Adding "+incompatible" shouldn't make a difference.
			if got := commitFromVersion(test.version+"+incompatible", test.dir); got != test.want {
				t.Errorf("+incompatible: got %s, want %s", got, test.want)
			}
		})
	}
}

var testWeb = map[string]string{
	// Package at root of a GitHub repo.
	"https://alice.org/pkg": `<head> <meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg"></head>`,
	// Package in sub-directory.
	"https://alice.org/pkg/sub": `<head> <meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg"><body>`,
	// Fallback to http.
	"http://alice.org/pkg/http": `<head> <meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">`,
	// Meta tag in sub-directory does not match meta tag at root.
	"https://alice.org/pkg/mismatch": `<head> <meta name="go-import" content="alice.org/pkg hg https://github.com/alice/pkg">`,
	// More than one matching meta tag.
	"http://alice.org/pkg/multiple": `<head> ` +
		`<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">` +
		`<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">`,
	// Package with go-source meta tag.
	"https://alice.org/pkg/source": `<head>` +
		`<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">` +
		`<meta name="go-source" content="alice.org/pkg http://alice.org/pkg http://alice.org/pkg{/dir} http://alice.org/pkg{/dir}?f={file}#Line{line}">`,
	"https://alice.org/pkg/ignore": `<head>` +
		`<title>Hello</title>` +
		// Unknown meta name
		`<meta name="go-junk" content="alice.org/pkg http://alice.org/pkg http://alice.org/pkg{/dir} http://alice.org/pkg{/dir}?f={file}#Line{line}">` +
		// go-source before go-meta
		`<meta name="go-source" content="alice.org/pkg http://alice.org/pkg http://alice.org/pkg{/dir} http://alice.org/pkg{/dir}?f={file}#Line{line}">` +
		// go-import tag for the package
		`<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">` +
		// go-import with wrong number of fields
		`<meta name="go-import" content="alice.org/pkg https://github.com/alice/pkg">` +
		// go-import with no fields
		`<meta name="go-import" content="">` +
		// go-source with wrong number of fields
		`<meta name="go-source" content="alice.org/pkg blah">` +
		// meta tag for a different package
		`<meta name="go-import" content="alice.org/other git https://github.com/alice/other">` +
		// meta tag for a different package
		`<meta name="go-import" content="alice.org/other git https://github.com/alice/other">` +
		`</head>` +
		// go-import outside of head
		`<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">`,

	// go-source repo defaults to go-import
	"http://alice.org/pkg/default": `<head>
		<meta name="go-import" content="alice.org/pkg git https://github.com/alice/pkg">
		<meta name="go-source" content="alice.org/pkg _ foo bar">
	</head>`,
	// Package at root of a Git repo.
	"https://bob.com/pkg": `<head> <meta name="go-import" content="bob.com/pkg git https://vcs.net/bob/pkg.git">`,
	// Package at in sub-directory of a Git repo.
	"https://bob.com/pkg/sub": `<head> <meta name="go-import" content="bob.com/pkg git https://vcs.net/bob/pkg.git">`,

	// Package with go-source meta tag, where {file} appears on the right of '#' in the file field URL template.
	"https://azul3d.org/examples/abs": `<!DOCTYPE html><html><head>` +
		`<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>` +
		`<meta name="go-import" content="azul3d.org/examples git https://github.com/azul3d/examples">` +
		`<meta name="go-source" content="azul3d.org/examples https://github.com/azul3d/examples https://gotools.org/azul3d.org/examples{/dir} https://gotools.org/azul3d.org/examples{/dir}#{file}-L{line}">` +
		`<meta http-equiv="refresh" content="0; url=https://godoc.org/azul3d.org/examples/abs">` +
		`</head>`,

	// Multiple go-import meta tags; one of which is a vgo-special mod vcs type
	"http://myitcv.io/blah2": `<!DOCTYPE html><html><head>` +
		`<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>` +
		`<meta name="go-import" content="myitcv.io/blah2 git https://github.com/myitcv/x">` +
		`<meta name="go-import" content="myitcv.io/blah2 mod https://raw.githubusercontent.com/myitcv/pubx/master">` +
		`</head>`,
}
