// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-replayers/httpreplay"
)

var (
	testTimeout = 2 * time.Second
	record      = flag.Bool("record", false, "record interactions with other systems, for replay")
)

func TestModuleInfo(t *testing.T) {
	client, done := newReplayClient(t, *record)
	defer done()

	check := func(t *testing.T, msg, got, want string) {
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

	for _, test := range []struct {
		desc                                              string
		modulePath, version, file                         string
		wantRepo, wantModule, wantFile, wantLine, wantRaw string
	}{
		{
			"standard library",
			"std", "v1.12.0", "bytes/buffer.go",

			"https://github.com/golang/go",
			"https://github.com/golang/go/tree/go1.12/src",
			"https://github.com/golang/go/blob/go1.12/src/bytes/buffer.go",
			"https://github.com/golang/go/blob/go1.12/src/bytes/buffer.go#L1",
			// The raw URLs for the standard library are relative to the repo root, not
			// the module directory.
			"",
		},
		{
			"old standard library",
			"std", "v1.3.0", "bytes/buffer.go",

			"https://github.com/golang/go",
			"https://github.com/golang/go/tree/go1.3/src/pkg",
			"https://github.com/golang/go/blob/go1.3/src/pkg/bytes/buffer.go",
			"https://github.com/golang/go/blob/go1.3/src/pkg/bytes/buffer.go#L1",
			// The raw URLs for the standard library are relative to the repo root, not
			// the module directory.
			"",
		},
		{
			"github module at repo root",
			"github.com/pkg/errors", "v0.8.1", "errors.go",

			"https://github.com/pkg/errors",
			"https://github.com/pkg/errors/tree/v0.8.1",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go#L1",
			"https://github.com/pkg/errors/raw/v0.8.1/errors.go",
		},
		{
			"github module not at repo root",
			"github.com/hashicorp/consul/sdk", "v0.2.0", "freeport/freeport.go",

			"https://github.com/hashicorp/consul",
			"https://github.com/hashicorp/consul/tree/sdk/v0.2.0/sdk",
			"https://github.com/hashicorp/consul/blob/sdk/v0.2.0/sdk/freeport/freeport.go",
			"https://github.com/hashicorp/consul/blob/sdk/v0.2.0/sdk/freeport/freeport.go#L1",
			"https://github.com/hashicorp/consul/raw/sdk/v0.2.0/sdk/freeport/freeport.go",
		},
		{
			"bitbucket",
			"bitbucket.org/plazzaro/kami", "v1.2.1", "defaults.go",

			"https://bitbucket.org/plazzaro/kami",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1/defaults.go",
			"https://bitbucket.org/plazzaro/kami/src/v1.2.1/defaults.go#lines-1",
			"https://bitbucket.org/plazzaro/kami/raw/v1.2.1/defaults.go",
		},
		{
			"incompatible",
			"github.com/airbrake/gobrake", "v3.5.1+incompatible", "gobrake.go",

			"https://github.com/airbrake/gobrake",
			"https://github.com/airbrake/gobrake/tree/v3.5.1",
			"https://github.com/airbrake/gobrake/blob/v3.5.1/gobrake.go",
			"https://github.com/airbrake/gobrake/blob/v3.5.1/gobrake.go#L1",
			"https://github.com/airbrake/gobrake/raw/v3.5.1/gobrake.go",
		},
		{
			"x/tools",
			"golang.org/x/tools", "v0.0.0-20190927191325-030b2cf1153e", "README.md",

			"https://github.com/golang/tools",
			"https://github.com/golang/tools/tree/030b2cf1153e",
			"https://github.com/golang/tools/blob/030b2cf1153e/README.md",
			"https://github.com/golang/tools/blob/030b2cf1153e/README.md#L1",
			"https://github.com/golang/tools/raw/030b2cf1153e/README.md",
		},
		{
			"x/tools/gopls",
			"golang.org/x/tools/gopls", "v0.4.0", "main.go",

			"https://github.com/golang/tools",
			"https://github.com/golang/tools/tree/gopls/v0.4.0/gopls",
			"https://github.com/golang/tools/blob/gopls/v0.4.0/gopls/main.go",
			"https://github.com/golang/tools/blob/gopls/v0.4.0/gopls/main.go#L1",
			"https://github.com/golang/tools/raw/gopls/v0.4.0/gopls/main.go",
		},
		{
			"googlesource.com",
			"go.googlesource.com/image.git", "v0.0.0-20190910094157-69e4b8554b2a", "math/fixed/fixed.go",

			"https://go.googlesource.com/image",
			"https://go.googlesource.com/image/+/69e4b8554b2a",
			"https://go.googlesource.com/image/+/69e4b8554b2a/math/fixed/fixed.go",
			"https://go.googlesource.com/image/+/69e4b8554b2a/math/fixed/fixed.go#1",
			"",
		},
		{
			"git.apache.org",
			"git.apache.org/thrift.git", "v0.12.0", "lib/go/thrift/client.go",

			"https://github.com/apache/thrift",
			"https://github.com/apache/thrift/tree/v0.12.0",
			"https://github.com/apache/thrift/blob/v0.12.0/lib/go/thrift/client.go",
			"https://github.com/apache/thrift/blob/v0.12.0/lib/go/thrift/client.go#L1",
			"https://github.com/apache/thrift/raw/v0.12.0/lib/go/thrift/client.go",
		},
		{
			"vanity for github",
			"cloud.google.com/go/spanner", "v1.0.0", "doc.go",

			"https://github.com/googleapis/google-cloud-go",
			"https://github.com/googleapis/google-cloud-go/tree/spanner/v1.0.0/spanner",
			"https://github.com/googleapis/google-cloud-go/blob/spanner/v1.0.0/spanner/doc.go",
			"https://github.com/googleapis/google-cloud-go/blob/spanner/v1.0.0/spanner/doc.go#L1",
			"https://github.com/googleapis/google-cloud-go/raw/spanner/v1.0.0/spanner/doc.go",
		},
		{
			"vanity for bitbucket",
			"badc0de.net/pkg/glagolitic", "v0.0.0-20180930175637-92f736eb02d6", "doc.go",

			"https://bitbucket.org/ivucica/go-glagolitic",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6/doc.go",
			"https://bitbucket.org/ivucica/go-glagolitic/src/92f736eb02d6/doc.go#lines-1",
			"https://bitbucket.org/ivucica/go-glagolitic/raw/92f736eb02d6/doc.go",
		},
		{
			"vanity for googlesource.com",
			"cuelang.org/go", "v0.0.9", "cuego/doc.go",

			"https://cue.googlesource.com/cue",
			"https://cue.googlesource.com/cue/+/v0.0.9",
			"https://cue.googlesource.com/cue/+/v0.0.9/cuego/doc.go",
			"https://cue.googlesource.com/cue/+/v0.0.9/cuego/doc.go#1",
			"",
		},
		{
			"gitlab.com",
			"gitlab.com/akita/akita", "v1.4.1", "event.go",

			"https://gitlab.com/akita/akita",
			"https://gitlab.com/akita/akita/tree/v1.4.1",
			"https://gitlab.com/akita/akita/blob/v1.4.1/event.go",
			"https://gitlab.com/akita/akita/blob/v1.4.1/event.go#L1",
			"https://gitlab.com/akita/akita/raw/v1.4.1/event.go",
		},
		{
			"other gitlab",
			"gitlab.66xue.com/daihao/logkit", "v0.1.18", "color.go",

			"https://gitlab.66xue.com/daihao/logkit",
			"https://gitlab.66xue.com/daihao/logkit/tree/v0.1.18",
			"https://gitlab.66xue.com/daihao/logkit/blob/v0.1.18/color.go",
			"https://gitlab.66xue.com/daihao/logkit/blob/v0.1.18/color.go#L1",
			"https://gitlab.66xue.com/daihao/logkit/raw/v0.1.18/color.go",
		},
		{
			"gitee.com",
			"gitee.com/Billcoding/gotypes", "v0.1.0", "type.go",

			"https://gitee.com/Billcoding/gotypes",
			"https://gitee.com/Billcoding/gotypes/tree/v0.1.0",
			"https://gitee.com/Billcoding/gotypes/blob/v0.1.0/type.go",
			"https://gitee.com/Billcoding/gotypes/blob/v0.1.0/type.go#L1",
			"https://gitee.com/Billcoding/gotypes/raw/v0.1.0/type.go",
		},
		{
			"v2 as a branch",
			"github.com/jrick/wsrpc/v2", "v2.1.1", "rpc.go",

			"https://github.com/jrick/wsrpc",
			"https://github.com/jrick/wsrpc/tree/v2.1.1",
			"https://github.com/jrick/wsrpc/blob/v2.1.1/rpc.go",
			"https://github.com/jrick/wsrpc/blob/v2.1.1/rpc.go#L1",
			"https://github.com/jrick/wsrpc/raw/v2.1.1/rpc.go",
		},
		{
			"v2 as subdirectory",
			"gitlab.com/akita/akita/v2", "v2.0.0-rc.2", "event.go",

			"https://gitlab.com/akita/akita",
			"https://gitlab.com/akita/akita/tree/v2.0.0-rc.2/v2",
			"https://gitlab.com/akita/akita/blob/v2.0.0-rc.2/v2/event.go",
			"https://gitlab.com/akita/akita/blob/v2.0.0-rc.2/v2/event.go#L1",
			"https://gitlab.com/akita/akita/raw/v2.0.0-rc.2/v2/event.go",
		},
		{
			"gopkg.in, one element",
			"gopkg.in/yaml.v2", "v2.2.2", "yaml.go",

			"https://github.com/go-yaml/yaml",
			"https://github.com/go-yaml/yaml/tree/v2.2.2",
			"https://github.com/go-yaml/yaml/blob/v2.2.2/yaml.go",
			"https://github.com/go-yaml/yaml/blob/v2.2.2/yaml.go#L1",
			"https://github.com/go-yaml/yaml/raw/v2.2.2/yaml.go",
		},
		{
			"gopkg.in, two elements",
			"gopkg.in/boltdb/bolt.v1", "v1.3.0", "doc.go",

			"https://github.com/boltdb/bolt",
			"https://github.com/boltdb/bolt/tree/v1.3.0",
			"https://github.com/boltdb/bolt/blob/v1.3.0/doc.go",
			"https://github.com/boltdb/bolt/blob/v1.3.0/doc.go#L1",
			"https://github.com/boltdb/bolt/raw/v1.3.0/doc.go",
		},
		{
			"gonum.org",
			"gonum.org/v1/gonum", "v0.6.1", "doc.go",

			"https://github.com/gonum/gonum",
			"https://github.com/gonum/gonum/tree/v0.6.1",
			"https://github.com/gonum/gonum/blob/v0.6.1/doc.go",
			"https://github.com/gonum/gonum/blob/v0.6.1/doc.go#L1",
			"https://github.com/gonum/gonum/raw/v0.6.1/doc.go",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			info, err := ModuleInfo(context.Background(), &Client{client}, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}

			check(t, "repo", info.repoURL, test.wantRepo)
			check(t, "module", info.ModuleURL(), test.wantModule)
			check(t, "file", info.FileURL(test.file), test.wantFile)
			check(t, "line", info.LineURL(test.file, 1), test.wantLine)
			if test.wantRaw != "" {
				check(t, "raw", info.RawURL(test.file), test.wantRaw)
			}
		})
	}

	t.Run("stdlib-raw", func(t *testing.T) {
		// Test raw URLs from the standard library, which are a special case.
		info, err := ModuleInfo(context.Background(), &Client{client}, "std", "v1.13.3")
		if err != nil {
			t.Fatal(err)
		}
		const (
			file = "doc/gopher/fiveyears.jpg"
			want = "https://github.com/golang/go/raw/go1.13.3/doc/gopher/fiveyears.jpg"
		)
		check(t, "raw", info.RawURL(file), want)
	})
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
		{"github.com/a/b/c/>$", "github.com/a/b", "c/&gt;$"},
	} {
		t.Run(test.in, func(t *testing.T) {
			gotRepo, gotSuffix, _, err := matchStatic(test.in)
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
			Timeout:   testTimeout,
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
			// Has a go-source tag, but we can't use the templates.
			&Info{
				repoURL:   "http://alice.org/pkg",
				moduleDir: "source",
				commit:    "source/v1.2.3",
				// empty templates
			},
		},

		{
			"alice.org/pkg/ignore",
			// Stop at the first go-source.
			&Info{
				repoURL:   "http://alice.org/pkg",
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
				repoURL:   "https://vcs.net/bob/pkg.git",
				moduleDir: "",
				commit:    "v1.2.3",
				// empty templates
			},
		},
		{
			"bob.com/pkg/sub",
			&Info{
				repoURL:   "https://vcs.net/bob/pkg.git",
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
			"bob.com/bad/github",
			&Info{
				repoURL:   "https://github.com/bob/bad/&#34;&gt;$",
				moduleDir: "",
				commit:    "v1.2.3",
				templates: githubURLTemplates,
			},
		},
		{

			"bob.com/bad/apache",
			&Info{
				repoURL:   "https://git.apache.org/&gt;$",
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
	client := NewClient(testTimeout)
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
			"v1.2.3", "foo/v1",
			"foo/v1/v1.2.3", // don't remove "/vN" if N = 1
		},
		{
			"v1.2.3", "v1", // ditto
			"v1/v1.2.3",
		},
		{
			"v3.1.0", "foo/v3",
			"foo/v3.1.0", // do remove "/v2" and higher
		},
		{
			"v3.1.0", "v3",
			"v3.1.0", // ditto
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
	// Bad repo URLs.
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
