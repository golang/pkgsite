// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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

			info, err := ModuleInfo(test.modulePath, test.version)
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
