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

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

// Module represents a module version used to generate testdata.
type Module struct {
	ModulePath string
	Version    string
	Files      map[string]string
	zip        []byte
}

func goMod(m *Module) string {
	if m.Files == nil {
		return defaultGoMod(m.ModulePath)
	}
	if _, ok := m.Files["go.mod"]; !ok {
		return defaultGoMod(m.ModulePath)
	}
	return m.Files["go.mod"]
}

// SetupTestProxy creates a fake module proxy for testing using the given test
// version information. If modules is nil, it will default to hosting the
// modules in the testdata directory.
//
// It returns a function for tearing down the proxy after the test is completed
// and a Client for interacting with the test proxy.
func SetupTestProxy(t *testing.T, modules []*Module) (*Client, func()) {
	t.Helper()
	var cleaned []*Module
	for _, m := range modules {
		cleaned = append(cleaned, cleanTestModule(t, m))
	}
	return TestProxyServer(t, TestProxy(cleaned))
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

// TestProxy implements a fake proxy, hosting the given modules. If modules
// is nil, it serves the modules in the testdata directory.
func TestProxy(modules []*Module) *http.ServeMux {
	// Group different modules of a module together, so that we can get
	// the latest modules and create the list endpoint.
	byModule := make(map[string][]*Module)
	for _, m := range modules {
		byModule[m.ModulePath] = append(byModule[m.ModulePath], m)
	}

	mux := http.NewServeMux()
	for modPath, modVersions := range byModule {
		sort.Slice(modVersions, func(i, j int) bool {
			return semver.Compare(modVersions[i].Version, modVersions[j].Version) < 0
		})
		handle := func(path string, content io.ReadSeeker) {
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, path, time.Now(), content)
			})
		}
		latest := func(modVersions []*Module) string {
			return modVersions[len(modVersions)-1].Version
		}
		master := func(modVersions []*Module) string {
			// TODO(https://golang.org/issue/39985): master should return the
			// most recently published version, which is not necessarily the
			// latest version according to semver.
			return modVersions[len(modVersions)-1].Version
		}
		handle(fmt.Sprintf("/%s/@v/list", modPath), strings.NewReader(versionList(modVersions)))
		handle(fmt.Sprintf("/%s/@latest", modPath), strings.NewReader(defaultInfo(latest(modVersions))))
		handle(fmt.Sprintf("/%s/@v/master.info", modPath), strings.NewReader(defaultInfo(master(modVersions))))
		for _, m := range modVersions {
			handle(fmt.Sprintf("/%s/@v/%s.info", m.ModulePath, m.Version), strings.NewReader(defaultInfo(m.Version)))
			handle(fmt.Sprintf("/%s/@v/%s.mod", m.ModulePath, m.Version), strings.NewReader(goMod(m)))
			handle(fmt.Sprintf("/%s/@v/%s.zip", m.ModulePath, m.Version), bytes.NewReader(m.zip))
		}
	}
	return mux
}

const versionTime = "2019-01-30T00:00:00Z"

func defaultInfo(version string) string {
	return fmt.Sprintf("{\n\t\"Version\": %q,\n\t\"Time\": %q\n}", version, versionTime)
}

func versionList(modVersions []*Module) string {
	var vList []string
	for _, v := range modVersions {
		vList = append(vList, v.Version)
	}
	return strings.Join(vList, "\n")
}

// defaultGoMod creates a bare-bones go.mod contents.
func defaultGoMod(modulePath string) string {
	return fmt.Sprintf("module %s\n\ngo 1.12", modulePath)
}

func cleanTestModule(t *testing.T, m *Module) *Module {
	t.Helper()
	if m.Version == "" {
		m.Version = "v1.0.0"
	}

	files := map[string]string{}
	for path, contents := range m.Files {
		p := m.ModulePath + "@" + m.Version + "/" + path
		files[p] = contents
	}
	zip, err := testhelper.ZipContents(files)
	if err != nil {
		t.Fatal(err)
	}
	m.zip = zip
	return m
}
