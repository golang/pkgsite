// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	Zip        []byte
	GoMod      string
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
			handle(fmt.Sprintf("/%s/@v/%s.mod", m, v.Version), strings.NewReader(goMod))
			handle(fmt.Sprintf("/%s/@v/%s.info", m, v.Version), strings.NewReader(defaultInfo(v.Version)))
			handle(fmt.Sprintf("/%s/@v/%s.zip", m, v.Version), bytes.NewReader(v.Zip))
		}
	}
	return mux
}

// defaultTestVersions creates TestVersions for the modules contained in the
// testdata directory.
func defaultTestVersions() []*TestVersion {
	proxyDataDir := testhelper.TestDataPath("testdata/modproxy")

	absPath, err := filepath.Abs(proxyDataDir)
	if err != nil {
		panic(fmt.Sprintf("filepath.Abs(%q): %v", proxyDataDir, err))
	}

	var versions []*TestVersion
	for _, v := range [][]string{
		[]string{"my.mod/module", "v1.0.0"},
		[]string{"no.mod/module", "v1.0.0"},
		[]string{"nonredistributable.mod/module", "v1.0.0"},
		[]string{"emp.ty/module", "v1.0.0"},
		[]string{"rsc.io/quote", "v1.5.2"},
		[]string{"rsc.io/quote/v2", "v2.0.1"},
	} {
		rootDir := filepath.Join(absPath, "modules")
		bytes, err := zipFiles(rootDir, filepath.FromSlash(fmt.Sprintf("%s@%s", v[0], v[1])))
		if err != nil {
			panic(err)
		}

		versions = append(versions, &TestVersion{
			ModulePath: v[0],
			Version:    v[1],
			Zip:        bytes,
		})
	}

	return versions
}

// SetupTestProxy creates a fake module proxy for testing using the given test
// version information. If versions is nil, it will default to hosting the
// modules in the testdata directory.
//
// It returns a function for tearing down the proxy after the test is completed
// and a Client for interacting with the test proxy.
func SetupTestProxy(ctx context.Context, t *testing.T, versions []*TestVersion) (func(t *testing.T), *Client) {
	t.Helper()

	p := httptest.NewTLSServer(TestProxy(versions))

	client, err := New(p.URL)
	if err != nil {
		t.Fatalf("New(%q): %v", p.URL, err)
	}
	// override client.httpClient to skip TLS verification
	client.httpClient = testhelper.InsecureHTTPClient

	fn := func(t *testing.T) {
		p.Close()
	}
	return fn, client
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
