// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal/testhelper"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// TestVersion represents a module version to host in the fake proxy.
type TestVersion struct {
	ModulePath string
	Version    string
	GoMod      string
	Zip        []byte
}

// SetupTestProxy creates a fake module proxy for testing using the given test
// version information. If versions is nil, it will default to hosting the
// modules in the testdata directory.
//
// It returns a function for tearing down the proxy after the test is completed
// and a Client for interacting with the test proxy.
func SetupTestProxy(t *testing.T, versions []*TestVersion) (func(t *testing.T), *Client) {
	t.Helper()

	client, cleanup := TestProxyServer(t, TestProxy(versions))
	return func(*testing.T) { cleanup() }, client
}

// TestProxyServer starts serving proxyMux locally. It returns a client to the
// server and a function to shut down the server.
func TestProxyServer(t *testing.T, proxyMux *http.ServeMux) (*Client, func()) {
	// override client.httpClient to skip TLS verification
	httpClient, proxy, serverClose := testhelper.SetupTestClientAndServer(proxyMux)
	client, err := New(proxy.URL)
	if err != nil {
		t.Fatalf("New(%q): %v", proxy.URL, err)
	}
	client.httpClient = httpClient
	return client, serverClose
}

// TestProxy implements a fake proxy, hosting the given versions. If versions
// is nil, it serves the modules in the testdata directory.
func TestProxy(versions []*TestVersion) *http.ServeMux {
	const versionTime = "2019-01-30T00:00:00Z"

	if versions == nil {
		versions = defaultTestVersions()
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
		for _, v := range vs {
			goMod := v.GoMod
			if goMod == "" {
				goMod = defaultGoMod(m)
			}
			if strings.HasPrefix(m, stdlibProxyModulePathPrefix) {
				goVersion, err := goVersionForSemanticVersion(v.Version)
				if err != nil {
					panic(fmt.Sprintf("bad test data: v.Version = %q", v.Version))
				}
				handle(fmt.Sprintf("/%s/@v/%s.info", m, goVersion), strings.NewReader(defaultInfo(v.Version)))
			} else {
				handle(fmt.Sprintf("/%s/@v/%s.info", m, v.Version), strings.NewReader(defaultInfo(v.Version)))
			}
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
		t.Fatalf("testhelper.ZipContents(%v): %v,", nestedContents, err)
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

// defaultTestVersions creates TestVersions for the modules contained in the
// testdata directory.
func defaultTestVersions() []*TestVersion {
	proxyDataDir := testhelper.TestDataPath("testdata/modproxy")
	absPath, err := filepath.Abs(proxyDataDir)
	if err != nil {
		log.Fatalf(fmt.Sprintf("filepath.Abs(%q): %v", proxyDataDir, err))
	}

	var versions []*TestVersion
	for _, v := range [][]string{
		{"go.googlesource.com/go.git", "v1.12.5"},
		{"go.googlesource.com/go.git/src", "v1.13.0-beta.1"},
		{"go.googlesource.com/go.git/src/cmd", "v1.13.0-beta.1"},
		{"bad.mod/module", "v1.0.0"},
		{"emp.ty/module", "v1.0.0"},
		{"github.com/my/module", "v1.0.0"},
		{"no.mod/module", "v1.0.0"},
		{"nonredistributable.mod/module", "v1.0.0"},
		{"rsc.io/quote", "v1.5.2"},
		{"rsc.io/quote/v2", "v2.0.1"},
		{"build.constraints/module", "v1.0.0"},
	} {
		rootDir := filepath.Join(absPath, "modules")
		f := filepath.FromSlash(fmt.Sprintf("%s@%s", v[0], v[1]))
		bytes, err := zipFiles(rootDir, f)
		if err != nil {
			log.Fatalf(fmt.Sprintf("zipFiles(%q, %q): %v", rootDir, f, err))
		}

		versions = append(versions, &TestVersion{
			ModulePath: v[0],
			Version:    v[1],
			Zip:        bytes,
		})
	}
	return versions
}

func zipFiles(dir, moduleDir string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := writeZip(buf, dir, moduleDir); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZip(w io.Writer, rootDir, moduleDir string) error {
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	return filepath.Walk(filepath.Join(rootDir, moduleDir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileToZip, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("os.Open(%q): %v", path, err)
		}
		defer fileToZip.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("zipFileInfoHeader(%v): %v", info.Name(), err)
		}

		// Using FileInfoHeader() above only uses the basename of the file. If we want
		// to preserve the folder structure we can overwrite this with the full path.
		header.Name = strings.TrimPrefix(path, filepath.ToSlash(rootDir)+"/")
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("zipWriter.CreateHeader(%+v): %v", header, err)
		}

		if _, err = io.Copy(writer, fileToZip); err != nil {
			return fmt.Errorf("io.Copy(%v, %+v): %v", writer, fileToZip, err)
		}
		return nil
	})
}
