// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal/testing/testhelper"
	"golang.org/x/mod/semver"
)

// TestVersion represents a module version to host in the fake proxy.
// This is being deprecated in favor of TestModule.
type TestVersion struct {
	ModulePath string
	Version    string
	GoMod      string
	Zip        []byte
}

// TestModule represents a module version used to generate testdata.
type TestModule struct {
	ModulePath   string
	Version      string
	Files        map[string]string
	ExcludeGoMod bool
}

// SetupTestProxy creates a fake module proxy for testing using the given test
// version information. If versions is nil, it will default to hosting the
// modules in the testdata directory.
//
// It returns a function for tearing down the proxy after the test is completed
// and a Client for interacting with the test proxy.
func SetupTestProxy(t *testing.T, versions []*TestVersion) (*Client, func()) {
	t.Helper()
	return TestProxyServer(t, TestProxy(t, versions))
}

// TestProxyServer starts serving proxyMux locally. It returns a client to the
// server and a function to shut down the server.
func TestProxyServer(t *testing.T, proxyMux *http.ServeMux) (*Client, func()) {
	// override client.httpClient to skip TLS verification
	httpClient, proxy, serverClose := testhelper.SetupTestClientAndServer(proxyMux)
	client, err := New(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = httpClient
	return client, serverClose
}

// TestProxy implements a fake proxy, hosting the given versions. If versions
// is nil, it serves the modules in the testdata directory.
func TestProxy(t *testing.T, versions []*TestVersion) *http.ServeMux {
	const versionTime = "2019-01-30T00:00:00Z"

	if versions == nil {
		modules := defaultTestModules()
		for _, m := range modules {
			versions = append(versions, NewTestVersion(t, m.ModulePath, m.Version, m.Files))
		}
	}

	defaultInfo := func(version string) string {
		return fmt.Sprintf("{\n\t\"Version\": %q,\n\t\"Time\": %q\n}", version, versionTime)
	}

	byModule := make(map[string][]*TestVersion)
	for _, v := range versions {
		byModule[v.ModulePath] = append(byModule[v.ModulePath], v)
	}

	mux := http.NewServeMux()
	for m, vs := range byModule {
		sort.Slice(vs, func(i, j int) bool {
			return semver.Compare(vs[i].Version, vs[j].Version) < 0
		})
		var vList []string
		for _, v := range vs {
			vList = append(vList, v.Version)
		}
		handle := func(path string, content io.ReadSeeker) {
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, path, time.Now(), content)
			})
		}
		handle(fmt.Sprintf("/%s/@v/list", m), strings.NewReader(strings.Join(vList, "\n")))
		handle(fmt.Sprintf("/%s/@latest", m), strings.NewReader(defaultInfo(vs[len(vs)-1].Version)))
		for _, v := range vs {
			goMod := v.GoMod
			if goMod == "" {
				goMod = defaultGoMod(m)
			}
			handle(fmt.Sprintf("/%s/@v/%s.info", m, v.Version), strings.NewReader(defaultInfo(v.Version)))
			handle(fmt.Sprintf("/%s/@v/%s.mod", m, v.Version), strings.NewReader(goMod))
			handle(fmt.Sprintf("/%s/@v/%s.zip", m, v.Version), bytes.NewReader(v.Zip))
		}
	}
	return mux
}

// NewTestVersion creates a new TestVersion from the given contents.
func NewTestVersion(t *testing.T, modulePath, version string, contents map[string]string) *TestVersion {
	t.Helper()
	nestedContents := make(map[string]string)
	for name, content := range contents {
		nestedContents[fmt.Sprintf("%s@%s/%s", modulePath, version, name)] = content
	}
	zip, err := testhelper.ZipContents(nestedContents)
	if err != nil {
		t.Fatal(err)
	}
	return &TestVersion{
		ModulePath: modulePath,
		Version:    version,
		Zip:        zip,
		GoMod:      contents["go.mod"],
	}
}

// defaultGoMod creates a bare-bones go.mod contents.
func defaultGoMod(modulePath string) string {
	return fmt.Sprintf("module %s\n\ngo 1.12", modulePath)
}
