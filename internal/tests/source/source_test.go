// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"context"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-replayers/httpreplay"
	"golang.org/x/pkgsite/internal/source"
)

var (
	record = flag.Bool("record", false, "record interactions with other systems, for replay")
)

func TestModuleInfo(t *testing.T) {
	client, done := newReplayClient(t, *record)
	defer done()

	// Test names where we don't replay/record actual URLs.
	skipReplayTests := map[string]bool{
		// On 5-Jan-2022, gitee.com took too long to respond, so it wasn't possible
		// to record the results.
		"gitee.com": true,
	}

	check := func(t *testing.T, msg, got, want string, skipReplay bool) {
		t.Helper()
		if got != want {
			t.Fatalf("%s:\ngot  %s\nwant %s", msg, got, want)
		}
		if !skipReplay {
			res, err := client.Head(got)
			if err != nil {
				t.Fatalf("%s: %v", got, err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("%s: %v", got, res.Status)
			}
		}
	}

	for _, test := range []struct {
		desc                                              string
		modulePath, version, file                         string
		wantRepo, wantModule, wantFile, wantLine, wantRaw string
	}{
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
			"github module with VCS suffix",
			"github.com/pkg/errors.git", "v0.8.1", "errors.go",

			"https://github.com/pkg/errors",
			"https://github.com/pkg/errors/tree/v0.8.1",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go",
			"https://github.com/pkg/errors/blob/v0.8.1/errors.go#L1",
			"https://github.com/pkg/errors/raw/v0.8.1/errors.go",
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
			"golang x-tools",
			"golang.org/x/tools", "v0.0.0-20190927191325-030b2cf1153e", "README.md",

			"https://cs.opensource.google/go/x/tools",
			"https://cs.opensource.google/go/x/tools/+/030b2cf1:",
			"https://cs.opensource.google/go/x/tools/+/030b2cf1:README.md",
			"https://cs.opensource.google/go/x/tools/+/030b2cf1:README.md;l=1",
			"https://github.com/golang/tools/raw/030b2cf1/README.md",
		},
		{
			"golang x-tools-gopls",
			"golang.org/x/tools/gopls", "v0.4.0", "main.go",

			"https://cs.opensource.google/go/x/tools",
			"https://cs.opensource.google/go/x/tools/+/gopls/v0.4.0:gopls",
			"https://cs.opensource.google/go/x/tools/+/gopls/v0.4.0:gopls/main.go",
			"https://cs.opensource.google/go/x/tools/+/gopls/v0.4.0:gopls/main.go;l=1",
			"https://github.com/golang/tools/raw/gopls/v0.4.0/gopls/main.go",
		},
		{
			"golang dl",
			"golang.org/dl", "c5c89f6c", "go1.16/main.go",

			"https://cs.opensource.google/go/dl",
			"https://cs.opensource.google/go/dl/+/c5c89f6c:",
			"https://cs.opensource.google/go/dl/+/c5c89f6c:go1.16/main.go",
			"https://cs.opensource.google/go/dl/+/c5c89f6c:go1.16/main.go;l=1",
			"https://github.com/golang/dl/raw/c5c89f6c/go1.16/main.go",
		},
		{
			"golang x-image",
			"golang.org/x/image", "v0.0.0-20190910094157-69e4b8554b2a", "math/fixed/fixed.go",

			"https://cs.opensource.google/go/x/image",
			"https://cs.opensource.google/go/x/image/+/69e4b855:",
			"https://cs.opensource.google/go/x/image/+/69e4b855:math/fixed/fixed.go",
			"https://cs.opensource.google/go/x/image/+/69e4b855:math/fixed/fixed.go;l=1",
			"https://github.com/golang/image/raw/69e4b855/math/fixed/fixed.go",
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
			"go.niquid.tech/civic-sip-api", "v0.2.0", "client.go",

			"https://bitbucket.org/niquid/civic-sip-api.git",
			"https://bitbucket.org/niquid/civic-sip-api.git/src/v0.2.0",
			"https://bitbucket.org/niquid/civic-sip-api.git/src/v0.2.0/client.go",
			"https://bitbucket.org/niquid/civic-sip-api.git/src/v0.2.0/client.go#lines-1",
			"https://bitbucket.org/niquid/civic-sip-api.git/raw/v0.2.0/client.go",
		},
		{
			"vanity for googlesource.com",
			"go.chromium.org/goma/server", "v0.0.23", "log/log.go",

			"https://chromium.googlesource.com/infra/goma/server",
			"https://chromium.googlesource.com/infra/goma/server/+/v0.0.23",
			"https://chromium.googlesource.com/infra/goma/server/+/v0.0.23/log/log.go",
			"https://chromium.googlesource.com/infra/goma/server/+/v0.0.23/log/log.go#1",
			"",
		},
		{
			"gitlab.com",
			"gitlab.com/tozd/go/errors", "v0.3.0", "errors.go",

			"https://gitlab.com/tozd/go/errors",
			"https://gitlab.com/tozd/go/errors/-/tree/v0.3.0",
			"https://gitlab.com/tozd/go/errors/-/blob/v0.3.0/errors.go",
			"https://gitlab.com/tozd/go/errors/-/blob/v0.3.0/errors.go#L1",
			"https://gitlab.com/tozd/go/errors/-/raw/v0.3.0/errors.go",
		},
		{
			"other gitlab",
			"gitlab.void-ptr.org/go/nu40c16", "v0.1.2", "nu40c16.go",

			"https://gitlab.void-ptr.org/go/nu40c16",
			"https://gitlab.void-ptr.org/go/nu40c16/-/tree/v0.1.2",
			"https://gitlab.void-ptr.org/go/nu40c16/-/blob/v0.1.2/nu40c16.go",
			"https://gitlab.void-ptr.org/go/nu40c16/-/blob/v0.1.2/nu40c16.go#L1",
			"https://gitlab.void-ptr.org/go/nu40c16/-/raw/v0.1.2/nu40c16.go",
		},
		{
			"gitee.com",
			"gitee.com/eden-framework/plugins", "v0.0.7", "file.go",

			"https://gitee.com/eden-framework/plugins",
			"https://gitee.com/eden-framework/plugins/tree/v0.0.7",
			"https://gitee.com/eden-framework/plugins/blob/v0.0.7/file.go",
			"https://gitee.com/eden-framework/plugins/blob/v0.0.7/file.go#L1",
			"https://gitee.com/eden-framework/plugins/raw/v0.0.7/file.go",
		},
		{
			"sourcehut",
			"gioui.org", "v0.0.0-20200726090130-3b95e2918359", "op/op.go",

			"https://git.sr.ht/~eliasnaur/gio",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359/op/op.go",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359/op/op.go#L1",
			"https://git.sr.ht/~eliasnaur/gio/blob/3b95e2918359/op/op.go",
		},
		{
			"sourcehut nested",
			"gioui.org/app", "v0.0.0-20200726090130-3b95e2918359", "app.go",

			"https://git.sr.ht/~eliasnaur/gio",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359/app",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359/app/app.go",
			"https://git.sr.ht/~eliasnaur/gio/tree/3b95e2918359/app/app.go#L1",
			"https://git.sr.ht/~eliasnaur/gio/blob/3b95e2918359/app/app.go",
		},
		{
			"git.fd.io tag",
			"git.fd.io/govpp", "v0.3.5", "doc.go",

			"https://git.fd.io/govpp",
			"https://git.fd.io/govpp/tree/?h=v0.3.5",
			"https://git.fd.io/govpp/tree/doc.go?h=v0.3.5",
			"https://git.fd.io/govpp/tree/doc.go?h=v0.3.5#n1",
			"https://git.fd.io/govpp/plain/doc.go?h=v0.3.5",
		},
		{
			"git.fd.io hash",
			"git.fd.io/govpp", "v0.0.0-20200726090130-f04939006063", "doc.go",

			"https://git.fd.io/govpp",
			"https://git.fd.io/govpp/tree/?id=f04939006063",
			"https://git.fd.io/govpp/tree/doc.go?id=f04939006063",
			"https://git.fd.io/govpp/tree/doc.go?id=f04939006063#n1",
			"https://git.fd.io/govpp/plain/doc.go?id=f04939006063",
		},
		{
			"gitea",
			"gitea.com/chenli/reverse", "v0.1.2", "main.go",

			"https://gitea.com/chenli/reverse",
			"https://gitea.com/chenli/reverse/src/tag/v0.1.2",
			"https://gitea.com/chenli/reverse/src/tag/v0.1.2/main.go",
			"https://gitea.com/chenli/reverse/src/tag/v0.1.2/main.go#L1",
			"https://gitea.com/chenli/reverse/raw/tag/v0.1.2/main.go",
		},
		{
			"gogs",
			"gogs.buffalo-robot.com/zouhy/micro", "v0.4.2", "go.mod",

			"https://gogs.buffalo-robot.com/zouhy/micro",
			"https://gogs.buffalo-robot.com/zouhy/micro/src/v0.4.2",
			"https://gogs.buffalo-robot.com/zouhy/micro/src/v0.4.2/go.mod",
			"https://gogs.buffalo-robot.com/zouhy/micro/src/v0.4.2/go.mod#L1",
			"https://gogs.buffalo-robot.com/zouhy/micro/raw/v0.4.2/go.mod",
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
			"github.com/gonutz/w32/v2", "v2.2.3", "com.go",

			"https://github.com/gonutz/w32",
			"https://github.com/gonutz/w32/tree/v2.2.3/v2",
			"https://github.com/gonutz/w32/blob/v2.2.3/v2/com.go",
			"https://github.com/gonutz/w32/blob/v2.2.3/v2/com.go#L1",
			"https://github.com/gonutz/w32/raw/v2.2.3/v2/com.go",
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
		{
			"custom with gotools at repo root",
			"dmitri.shuralyov.com/gpu/mtl", "v0.0.0-20191203043605-d42048ed14fd", "mtl.go",

			"https://dmitri.shuralyov.com/gpu/mtl/...",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl?rev=d42048ed14fd",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl?rev=d42048ed14fd#mtl.go",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl?rev=d42048ed14fd#mtl.go-L1",
			"",
		},
		{
			"custom with gotools in subdir",
			"dmitri.shuralyov.com/gpu/mtl", "v0.0.0-20191203043605-d42048ed14fd", "example/movingtriangle/internal/coreanim/coreanim.go",

			"https://dmitri.shuralyov.com/gpu/mtl/...",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl?rev=d42048ed14fd",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl/example/movingtriangle/internal/coreanim?rev=d42048ed14fd#coreanim.go",
			"https://gotools.org/dmitri.shuralyov.com/gpu/mtl/example/movingtriangle/internal/coreanim?rev=d42048ed14fd#coreanim.go-L1",
			"",
		},
		{
			"go-source templates match gitea with transform",
			"opendev.org/airship/airshipctl", "v2.0.0-beta.1", "pkg/cluster/command.go",
			"https://opendev.org/airship/airshipctl",
			"https://opendev.org/airship/airshipctl/src/tag/v2.0.0-beta.1",
			"https://opendev.org/airship/airshipctl/src/tag/v2.0.0-beta.1/pkg/cluster/command.go",
			"https://opendev.org/airship/airshipctl/src/tag/v2.0.0-beta.1/pkg/cluster/command.go#L1",
			"",
		},
		{
			"go-source templates match gitea without transform",
			"git.borago.de/Marco/gqltest", "v0.0.18", "go.mod",
			"https://git.borago.de/Marco/gqltest",
			"https://git.borago.de/Marco/gqltest/src/v0.0.18",
			"https://git.borago.de/Marco/gqltest/src/v0.0.18/go.mod",
			"https://git.borago.de/Marco/gqltest/src/v0.0.18/go.mod#L1",
			"https://git.borago.de/Marco/gqltest/raw/v0.0.18/go.mod",
		},
		{
			"go-source templates match gitlab2",
			"git.pluggableideas.com/destrealm/3rdparty/go-yaml", "v2.2.6", "go.mod",
			"https://git.pluggableideas.com/destrealm/3rdparty/go-yaml",
			"https://git.pluggableideas.com/destrealm/3rdparty/go-yaml/-/tree/v2.2.6",
			"https://git.pluggableideas.com/destrealm/3rdparty/go-yaml/-/blob/v2.2.6/go.mod",
			"https://git.pluggableideas.com/destrealm/3rdparty/go-yaml/-/blob/v2.2.6/go.mod#L1",
			"https://git.pluggableideas.com/destrealm/3rdparty/go-yaml/-/raw/v2.2.6/go.mod",
		},
		{
			"go-source templates match fdio",
			"golang.zx2c4.com/wireguard/windows", "v0.3.4", "go.mod",
			"https://git.zx2c4.com/wireguard-windows",
			"https://git.zx2c4.com/wireguard-windows/tree/?h=v0.3.4",
			"https://git.zx2c4.com/wireguard-windows/tree/go.mod?h=v0.3.4",
			"https://git.zx2c4.com/wireguard-windows/tree/go.mod?h=v0.3.4#n1",
			"https://git.zx2c4.com/wireguard-windows/plain/go.mod?h=v0.3.4",
		},
		{
			"go-source templates match blitiri.com.ar",
			"blitiri.com.ar/go/log", "v1.1.0", "go.mod",
			"https://blitiri.com.ar/git/r/log",
			"https://blitiri.com.ar/git/r/log/b/master/t",
			"https://blitiri.com.ar/git/r/log/b/master/t/f=go.mod.html",
			"https://blitiri.com.ar/git/r/log/b/master/t/f=go.mod.html#line-1",
			"",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			info, err := source.ModuleInfo(context.Background(), source.NewClient(client), test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}

			skip := skipReplayTests[test.desc]
			check(t, "repo", info.RepoURL(), test.wantRepo, skip)
			check(t, "module", info.ModuleURL(), test.wantModule, skip)
			check(t, "file", info.FileURL(test.file), test.wantFile, skip)
			check(t, "line", info.LineURL(test.file, 1), test.wantLine, skip)
			if test.wantRaw != "" {
				check(t, "raw", info.RawURL(test.file), test.wantRaw, skip)
			}
		})
	}
}

func TestNewStdlibInfo(t *testing.T) {
	client, done := newReplayClient(t, *record)
	defer done()

	check := func(t *testing.T, msg, got, want string) {
		t.Helper()
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
			"https://cs.opensource.google/go/go",
			"https://cs.opensource.google/go/go/+/go1.12:src",
			"https://cs.opensource.google/go/go/+/go1.12:src/bytes/buffer.go",
			"https://cs.opensource.google/go/go/+/go1.12:src/bytes/buffer.go;l=1",
			// The raw URLs for the standard library are relative to the repo root, not
			// the module directory.
			"",
		},
		{
			"old standard library",
			"std", "v1.3.0", "bytes/buffer.go",
			"https://cs.opensource.google/go/go",
			"https://cs.opensource.google/go/go/+/go1.3:src/pkg",
			"https://cs.opensource.google/go/go/+/go1.3:src/pkg/bytes/buffer.go",
			"https://cs.opensource.google/go/go/+/go1.3:src/pkg/bytes/buffer.go;l=1",
			// The raw URLs for the standard library are relative to the repo root, not
			// the module directory.
			"",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			info, err := source.NewStdlibInfo(test.version)
			if err != nil {
				t.Fatal(err)
			}

			check(t, "repo", info.RepoURL(), test.wantRepo)
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
		info, err := source.NewStdlibInfo("v1.13.3")
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
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skipf("skipping test: see issue #59718: listen on localhost fails on %s", runtime.GOOS)
	}
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
