// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMatchStatic(t *testing.T) {
	for _, test := range []struct {
		in                   string
		wantRepo, wantSuffix string
	}{
		{"github.com/a/b", "github.com/a/b", ""},
		{"bitbucket.org/a/b", "bitbucket.org/a/b", ""},
		{"github.com/a/b/c/d", "github.com/a/b", "c/d"},
		{"bitbucket.org/a/b/c/d", "bitbucket.org/a/b", "c/d"},
		{"foo.googlesource.com/a/b/c", "foo.googlesource.com/a/b/c", ""},
		{"foo.googlesource.com/a/b/c.git", "foo.googlesource.com/a/b/c", ""},
		{"foo.googlesource.com/a/b/c.git/d", "foo.googlesource.com/a/b/c", "d"},
		{"git.com/repo.git", "git.com/repo", ""},
		{"git.com/repo.git/dir", "git.com/repo", "dir"},
		{"mercurial.com/repo.hg", "mercurial.com/repo", ""},
		{"mercurial.com/repo.hg/dir", "mercurial.com/repo", "dir"},
	} {
		t.Run(test.in, func(t *testing.T) {
			gotRepo, gotSuffix, _, _, err := matchStatic(test.in)
			if err != nil {
				t.Fatal(err)
			}
			if gotRepo != test.wantRepo || gotSuffix != test.wantSuffix {
				t.Errorf("got %q, %q; want %q, %q", gotRepo, gotSuffix, test.wantRepo, test.wantSuffix)
			}
		})
	}
}

// This test adapted from gddo/gosrc/gosrc_test.go:TestGetDynamic.
func TestModuleInfoDynamic(t *testing.T) {
	// For this test, fake the HTTP requests so we can cover cases that may not appear in the wild.
	client := &Client{
		httpClient: &http.Client{
			Transport: testTransport(testWeb),
		},
	}
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
				repoURL:   "https://github.com/alice/pkg",
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/sub",
			&Info{
				repoURL:   "https://github.com/alice/pkg",
				moduleDir: "sub",
				commit:    "sub/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/http",
			&Info{
				repoURL:   "https://github.com/alice/pkg",
				moduleDir: "http",
				commit:    "http/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/source",
			// Has a go-source tag; we try to use the templates.
			&Info{
				repoURL:   "http://alice.org/pkg",
				moduleDir: "source",
				commit:    "source/v1.2.3",
				templates: urlTemplates{
					Repo:      "http://alice.org/pkg",
					Directory: "http://alice.org/pkg/{dir}",
					File:      "http://alice.org/pkg/{dir}?f={file}",
					Line:      "http://alice.org/pkg/{dir}?f={file}#Line{line}",
				},
			},
		},

		{
			"alice.org/pkg/ignore",
			// Stop at the first go-source.
			&Info{
				repoURL:   "http://alice.org/pkg",
				moduleDir: "ignore",
				commit:    "ignore/v1.2.3",
				templates: urlTemplates{
					Repo:      "http://alice.org/pkg",
					Directory: "http://alice.org/pkg/{dir}",
					File:      "http://alice.org/pkg/{dir}?f={file}",
					Line:      "http://alice.org/pkg/{dir}?f={file}#Line{line}",
				},
			},
		},
		{"alice.org/pkg/multiple", nil},
		{"alice.org/pkg/notfound", nil},
		{
			"bob.com/pkg",
			&Info{
				// The go-import tag's repo root ends in ".git", but according to the spec
				// there should not be a .vcs suffix, so we include the ".git" in the repo URL.
				repoURL:   "https://vcs.net/bob/pkg",
				moduleDir: "",
				commit:    "v1.2.3",
				// empty templates
			},
		},
		{
			"bob.com/pkg/sub",
			&Info{
				repoURL:   "https://vcs.net/bob/pkg",
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
				repoURL:   "https://github.com/azul3d/examples",
				moduleDir: "abs",
				commit:    "abs/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"myitcv.io/blah2",
			// Ignore the "mod" vcs type.
			&Info{
				repoURL:   "https://github.com/myitcv/x",
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			"alice.org/pkg/default",
			&Info{
				repoURL:   "https://github.com/alice/pkg",
				moduleDir: "default",
				commit:    "default/v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{
			// Bad repo URLs. These are not escaped here, but they are whenever we render a template.
			"bob.com/bad/github",
			&Info{
				repoURL:   `https://github.com/bob/bad/">$`,
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{

			"bob.com/bad/apache",
			&Info{
				repoURL:   "https://git.apache.org/>$",
				moduleDir: "",
				commit:    "v1.2.3",
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
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(Info{}, urlTemplates{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRemoveVersionSuffix(t *testing.T) {
	for _, test := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"v1", "v1"},
		{"v2", ""},
		{"v17", ""},
		{"foo/bar", "foo/bar"},
		{"foo/bar/v1", "foo/bar/v1"},
		{"foo/bar.v2", "foo/bar.v2"},
		{"foo/bar/v2", "foo/bar"},
		{"foo/bar/v17", "foo/bar"},
	} {
		got := removeVersionSuffix(test.in)
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}
}

func TestAdjustVersionedModuleDirectory(t *testing.T) {
	ctx := context.Background()

	client := NewClient(http.DefaultClient)
	client.httpClient.Transport = testTransport(map[string]string{
		// Repo "branch" follows the "major branch" convention: versions 2 and higher
		// live in the same directory as versions 0 and 1, but on a different branch (or tag).
		"http://x.com/branch/v1.0.0/go.mod":         "", // v1 module at the root
		"http://x.com/branch/v2.0.0/go.mod":         "", // v2 module at the root
		"http://x.com/branch/dir/v1.0.0/dir/go.mod": "", // v1 module in a subdirectory
		"http://x.com/branch/dir/v2.0.0/dir/go.mod": "", // v2 module in a subdirectory
		// Repo "sub" follows the "major subdirectory" convention: versions 2 and higher
		// live in a "vN" subdirectory.
		"http://x.com/sub/v1.0.0/go.mod":            "", // v1 module at the root
		"http://x.com/sub/v2.0.0/v2/go.mod":         "", // v2 module at root/v2.
		"http://x.com/sub/dir/v1.0.0/dir/go.mod":    "", // v1 module in a subdirectory
		"http://x.com/sub/dir/v2.0.0/dir/v2/go.mod": "", // v2 module in subdirectory/v2
	})

	for _, test := range []struct {
		repo, moduleDir, commit string
		want                    string
	}{
		{
			// module path is x.com/branch
			"branch", "", "v1.0.0",
			"",
		},
		{
			// module path is x.com/branch/v2; remove the "v2" to get the module dir
			"branch", "v2", "v2.0.0",
			"",
		},
		{
			// module path is x.com/branch/dir
			"branch", "dir", "dir/v1.0.0",
			"dir",
		},
		{
			// module path is x.com/branch/dir/v2; remove the v2 to get the module dir
			"branch", "dir/v2", "dir/v2.0.0",
			"dir",
		},
		{
			// module path is x.com/sub
			"sub", "", "v1.0.0",
			"",
		},
		{
			// module path is x.com/sub/v2; do not remove the v2
			"sub", "v2", "v2.0.0",
			"v2",
		},
		{
			// module path is x.com/sub/dir
			"sub", "dir", "dir/v1.0.0",
			"dir",
		},
		{
			// module path is x.com/sub/dir/v2; do not remove the v2
			"sub", "dir/v2", "dir/v2.0.0",
			"dir/v2",
		},
	} {
		t.Run(test.repo+","+test.moduleDir+","+test.commit, func(t *testing.T) {
			info := &Info{
				repoURL:   "http://x.com/" + test.repo,
				moduleDir: test.moduleDir,
				commit:    test.commit,
				templates: urlTemplates{File: "{repo}/{commit}/{file}"},
			}
			adjustVersionedModuleDirectory(ctx, client, info)
			got := info.moduleDir
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestCommitFromVersion(t *testing.T) {
	for _, test := range []struct {
		version, dir string
		wantCommit   string
		wantIsHash   bool
	}{
		{
			"v1.2.3", "",
			"v1.2.3", false,
		},
		{
			"v1.2.3", "foo",
			"foo/v1.2.3", false,
		},
		{
			"v1.2.3", "foo/v1",
			"foo/v1/v1.2.3", // don't remove "/vN" if N = 1
			false,
		},
		{
			"v1.2.3", "v1", // ditto
			"v1/v1.2.3", false,
		},
		{
			"v3.1.0", "foo/v3",
			"foo/v3.1.0", // do remove "/v2" and higher
			false,
		},
		{
			"v3.1.0", "v3",
			"v3.1.0", // ditto
			false,
		},
		{
			"v6.1.1-0.20190615154606-3a9541ec9974", "",
			"3a9541ec9974", true,
		},
		{
			"v6.1.1-0.20190615154606-3a9541ec9974", "foo",
			"3a9541ec9974", true,
		},
	} {
		t.Run(fmt.Sprintf("%s,%s", test.version, test.dir), func(t *testing.T) {
			check := func(v string) {
				gotCommit, gotIsHash := commitFromVersion(v, test.dir)
				if gotCommit != test.wantCommit {
					t.Errorf("%s commit: got %s, want %s", v, gotCommit, test.wantCommit)
				}
				if gotIsHash != test.wantIsHash {
					t.Errorf("%s isHash: got %t, want %t", v, gotIsHash, test.wantIsHash)
				}
			}

			check(test.version)
			// Adding "+incompatible" shouldn't make a difference.
			check(test.version + "+incompatible")
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
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	return resp, nil
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
	"https://bob.com/bad/github": `
		<head><meta name="go-import" content="bob.com/bad/github git https://github.com/bob/bad/&quot;&gt;$">`,
	"https://bob.com/bad/apache": `
		<head><meta name="go-import" content="bob.com/bad/apache git https://git.apache.org/&gt;$">`,
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

func TestJSON(t *testing.T) {
	for _, test := range []struct {
		in   *Info
		want string
	}{
		{
			nil,
			`null`,
		},
		{
			&Info{repoURL: "r", moduleDir: "m", commit: "c"},
			`{"RepoURL":"r","ModuleDir":"m","Commit":"c"}`,
		},
		{
			&Info{repoURL: "r", moduleDir: "m", commit: "c", templates: githubURLTemplates},
			`{"RepoURL":"r","ModuleDir":"m","Commit":"c","Kind":"github"}`,
		},
		{
			&Info{repoURL: "r", moduleDir: "m", commit: "c", templates: urlTemplates{File: "f"}},
			`{"RepoURL":"r","ModuleDir":"m","Commit":"c","Templates":{"Directory":"","File":"f","Line":"","Raw":""}}`,
		},
		{
			&Info{repoURL: "r", moduleDir: "m", commit: "c", templates: urlTemplates{Repo: "r", File: "f"}},
			`{"RepoURL":"r","ModuleDir":"m","Commit":"c","Templates":{"Repo":"r","Directory":"","File":"f","Line":"","Raw":""}}`,
		},
	} {
		bytes, err := json.Marshal(&test.in)
		if err != nil {
			t.Fatal(err)
		}
		got := string(bytes)
		if got != test.want {
			t.Errorf("%#v:\ngot  %s\nwant %s", test.in, got, test.want)
			continue
		}
		var out Info
		if err := json.Unmarshal(bytes, &out); err != nil {
			t.Fatal(err)
		}
		var want Info
		if test.in != nil {
			want = *test.in
		}
		if out != want {
			t.Errorf("got  %#v\nwant %#v", out, want)
		}
	}
}

func TestURLTemplates(t *testing.T) {
	// Check that templates contain the right variables.

	for _, p := range patterns {
		if strings.Contains(p.pattern, "blitiri") {
			continue
		}
		check := func(tmpl string, vars ...string) {
			if tmpl == "" {
				return
			}
			for _, v := range vars {
				w := "{" + v + "}"
				if !strings.Contains(tmpl, w) {
					t.Errorf("in pattern %s, template %q is missing %s", p.pattern, tmpl, w)
				}
			}
		}

		check(p.templates.Directory, "commit")
		check(p.templates.File, "commit")
		check(p.templates.Line, "commit", "line")
		check(p.templates.Raw, "commit", "file")
	}
}

func TestMatchLegacyTemplates(t *testing.T) {
	for _, test := range []struct {
		sm                     sourceMeta
		wantTemplates          urlTemplates
		wantTransformCommitNil bool
	}{
		{
			sm:                     sourceMeta{"", "", "", "https://git.blindage.org/21h/hcloud-dns/src/branch/master{/dir}/{file}#L{line}"},
			wantTemplates:          giteaURLTemplates,
			wantTransformCommitNil: false,
		},
		{
			sm:                     sourceMeta{"", "", "", "https://git.lastassault.de/sup/networkoverlap/-/blob/master{/dir}/{file}#L{line}"},
			wantTemplates:          gitlabURLTemplates,
			wantTransformCommitNil: true,
		},
		{
			sm:                     sourceMeta{"", "", "", "https://git.borago.de/Marco/gqltest/src/master{/dir}/{file}#L{line}"},
			wantTemplates:          giteaURLTemplates,
			wantTransformCommitNil: true,
		},
		{
			sm:                     sourceMeta{"", "", "", "https://git.zx2c4.com/wireguard-windows/tree{/dir}/{file}#n{line}"},
			wantTemplates:          fdioURLTemplates,
			wantTransformCommitNil: false,
		},
		{
			sm: sourceMeta{"", "", "unknown{/dir}", "unknown{/dir}/{file}#L{line}"},
			wantTemplates: urlTemplates{
				Repo:      "",
				Directory: "unknown/{dir}",
				File:      "unknown/{file}",
				Line:      "unknown/{file}#L{line}",
			},
			wantTransformCommitNil: true,
		},
	} {
		gotTemplates, gotTransformCommit := matchLegacyTemplates(context.Background(), &test.sm)
		gotNil := gotTransformCommit == nil
		if gotTemplates != test.wantTemplates || gotNil != test.wantTransformCommitNil {
			t.Errorf("%+v:\ngot  (%+v, %t)\nwant (%+v, %t)",
				test.sm, gotTemplates, gotNil, test.wantTemplates, test.wantTransformCommitNil)
		}
	}
}

func TestFilesInfo(t *testing.T) {
	info := FilesInfo("/Users/bob")

	check := func(got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	check(info.RepoURL(), "/files/Users/bob/")
	check(info.ModuleURL(), "/files/Users/bob/")
	check(info.FileURL("dir/a.go"), "/files/Users/bob/dir/a.go")
}
