// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testCase struct {
	proxy  *httptest.Server
	client *Client
}

func setupTestCase(t *testing.T) (func(t *testing.T), *testCase) {
	proxy := httptest.NewServer(http.FileServer(http.Dir("testdata/modproxy/proxy")))
	tc := testCase{
		proxy:  proxy,
		client: New(proxy.URL),
	}

	fn := func(t *testing.T) {
		proxy.Close()
	}
	return fn, &tc
}

func TestCleanURL(t *testing.T) {
	for raw, expected := range map[string]string{
		"http://localhost:7000/index": "http://localhost:7000/index",
		"http://host.com/":            "http://host.com",
		"http://host.com///":          "http://host.com",
	} {
		if got := cleanURL(raw); got != expected {
			t.Errorf("cleanURL(%q) = %q, want %q", raw, got, expected)
		}
	}
}

func TestGetInfo(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/module"
	version := "v1.0.0"
	info, err := testCase.client.GetInfo(name, version)
	if err != nil {
		t.Errorf("GetInfo(%q, %q) error: %v", name, version, err)
	}

	if info.Version != version {
		t.Errorf("VersionInfo.Version for GetInfo(%q, %q) = %q, want %q", name, version, info.Version, version)
	}

	expectedTime := time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
	if info.Time != expectedTime {
		t.Errorf("VersionInfo.Time for GetInfo(%q, %q) = %v, want %v", name, version, info.Time, expectedTime)
	}
}

func TestGetInfoVersionDoesNotExist(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/module"
	version := "v3.0.0"
	info, _ := testCase.client.GetInfo(name, version)
	if info != nil {
		t.Errorf("GetInfo(%q, %q) = %v, want %v", name, version, info, nil)
	}
}

func TestGetZip(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/module"
	version := "v1.0.0"
	zipReader, err := testCase.client.GetZip(name, version)
	if err != nil {
		t.Errorf("GetZip(%q, %q) error: %v", name, version, err)
	}

	expectedFiles := map[string]bool{
		"my/":                        true,
		"my/module@v1.0.0/":          true,
		"my/module@v1.0.0/LICENSE":   true,
		"my/module@v1.0.0/README.md": true,
		"my/module@v1.0.0/go.mod":    true,
	}
	if len(zipReader.File) != len(expectedFiles) {
		t.Errorf("GetZip(%q, %q) returned number of files: got %d, want %d",
			name, version, len(zipReader.File), len(expectedFiles))
	}

	for _, zipFile := range zipReader.File {
		if !expectedFiles[zipFile.Name] {
			t.Errorf("GetZip(%q, %q) returned unexpected file: %q", name,
				version, zipFile.Name)
		}
		delete(expectedFiles, zipFile.Name)
	}
}

func TestGetZipNonExist(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/nonexistmodule"
	version := "v1.0.0"
	expectedErr := fmt.Sprintf("http.Get(%q) returned response: %d (%q)",
		testCase.client.zipURL(name, version), 404, "404 Not Found")

	if _, err := testCase.client.GetZip(name, version); err.Error() != expectedErr {
		t.Errorf("GetZip(%q, %q) returned error %v, want %v", name, version, err, expectedErr)
	}
}
