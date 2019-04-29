// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// insecureHTTPClient is used to disable TLS verification when running against
// a test server.
var insecureHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

// SetupTestProxy creates a module proxy for testing using static files
// stored in internal/proxy/testdata/modproxy/proxy. It returns a function
// for tearing down the proxy after the test is completed and a Client for
// interacting with the test proxy.
func SetupTestProxy(ctx context.Context, t *testing.T) (func(t *testing.T), *Client) {
	t.Helper()

	proxyDataDir := "../proxy/testdata/modproxy"
	absPath, err := filepath.Abs(proxyDataDir)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", proxyDataDir, err)
	}

	p := httptest.NewTLSServer(http.FileServer(http.Dir(fmt.Sprintf("%s/proxy", absPath))))

	client, err := New(p.URL)
	if err != nil {
		t.Fatalf("New(%q): %v", p.URL, err)
	}
	// override client.httpClient to skip TLS verification
	client.httpClient = insecureHTTPClient

	for _, v := range [][]string{
		[]string{"my.mod/module", "v1.0.0"},
		[]string{"no.mod/module", "v1.0.0"},
		[]string{"emp.ty/module", "v1.0.0"},
		[]string{"rsc.io/quote", "v1.5.2"},
		[]string{"rsc.io/quote/v2", "v2.0.1"},
	} {
		zipfile := fmt.Sprintf("%s/proxy/%s/@v/%s.zip", absPath, v[0], v[1])
		zipDataDir := fmt.Sprintf("%s/modules/%s@%s", absPath, v[0], v[1])
		if _, err := ZipFiles(zipfile, zipDataDir, fmt.Sprintf("%s@%s", v[0], v[1])); err != nil {
			t.Fatalf("proxy.ZipFiles(%q): %v", zipDataDir, err)
		}

		if _, err := client.GetInfo(ctx, v[0], v[1]); err != nil {
			t.Fatalf("client.GetInfo(%q, %q): %v", v[0], v[1], err)
		}
	}

	fn := func(t *testing.T) {
		p.Close()
	}
	return fn, client
}

// ZipFiles compresses the files inside dir into a single zip archive file.
// zipfile is the output zip file's name. Files inside zipfile will all have
// prefix moduleDir. ZipFiles return a function to cleanup files that were
// created.
func ZipFiles(zipfile, dir, moduleDir string) (func() error, error) {
	cleanup := func() error {
		return os.Remove(zipfile)
	}

	newZipFile, err := os.Create(zipfile)
	if err != nil {
		return cleanup, fmt.Errorf("os.Create(%q): %v", zipfile, err)
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	return cleanup, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		header.Name = strings.TrimPrefix(strings.TrimPrefix(path, strings.TrimSuffix(dir, moduleDir)), "/")
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
