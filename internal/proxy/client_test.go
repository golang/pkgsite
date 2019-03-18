// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"testing"
	"time"
)

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
	teardownProxy, client := SetupTestProxy(t)
	defer teardownProxy(t)

	name := "my/module"
	version := "v1.0.0"
	info, err := client.GetInfo(name, version)
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
	teardownProxy, client := SetupTestProxy(t)
	defer teardownProxy(t)

	name := "my/module"
	version := "v3.0.0"
	info, _ := client.GetInfo(name, version)
	if info != nil {
		t.Errorf("GetInfo(%q, %q) = %v, want %v", name, version, info, nil)
	}
}

func TestGetZip(t *testing.T) {
	teardownProxy, client := SetupTestProxy(t)
	defer teardownProxy(t)

	name := "my/module"
	version := "v1.0.0"
	zipReader, err := client.GetZip(name, version)
	if err != nil {
		t.Errorf("GetZip(%q, %q) error: %v", name, version, err)
	}

	expectedFiles := map[string]bool{
		"my/module@v1.0.0/LICENSE":    true,
		"my/module@v1.0.0/README.md":  true,
		"my/module@v1.0.0/go.mod":     true,
		"my/module@v1.0.0/foo/foo.go": true,
		"my/module@v1.0.0/bar/bar.go": true,
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
	teardownProxy, client := SetupTestProxy(t)
	defer teardownProxy(t)

	name := "my/nonexistmodule"
	version := "v1.0.0"
	expectedErr := fmt.Sprintf("http.Get(%q) returned response: %d (%q)",
		client.zipURL(name, version), 404, "404 Not Found")

	if _, err := client.GetZip(name, version); err.Error() != expectedErr {
		t.Errorf("GetZip(%q, %q) returned error %v, want %v", name, version, err, expectedErr)
	}
}
