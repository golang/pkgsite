// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

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

func TestInfoURLAndZipURL(t *testing.T) {
	rawurl := "https://proxy.golang.org"
	client, err := New(rawurl)
	if err != nil {
		t.Fatalf("New(%q): %v", rawurl, err)
	}

	for _, tc := range []struct {
		name, path, version, wantInfoURL, wantZipURL string
	}{
		{
			name:        "module with tagged version",
			path:        "google.golang.org/api",
			version:     "v1.0.0",
			wantInfoURL: "google.golang.org/api/@v/v1.0.0.info",
			wantZipURL:  "google.golang.org/api/@v/v1.0.0.zip",
		},
		{
			name:        "must encode path and version",
			path:        "github.com/Azure/azure-sdk-for-go",
			version:     "v8.0.1-beta+incompatible",
			wantInfoURL: "github.com/!azure/azure-sdk-for-go/@v/v8.0.1-beta+incompatible.info",
			wantZipURL:  "github.com/!azure/azure-sdk-for-go/@v/v8.0.1-beta+incompatible.zip",
		},
		{
			name:        "standard library",
			path:        "std",
			version:     "v1.12.5",
			wantInfoURL: stdlibModulePathProxy + "/@v/go1.12.5.info",
			wantZipURL:  stdlibModulePathProxy + "/@v/v1.12.5.zip",
		},
		{
			name:        "standard library, patch version 0",
			path:        "std",
			version:     "v1.12.0",
			wantInfoURL: stdlibModulePathProxy + "/@v/go1.12.info",
			wantZipURL:  stdlibModulePathProxy + "/@v/v1.12.0.zip",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := fmt.Sprintf("%s/%s", client.url, tc.wantInfoURL)
			if got, _ := client.infoURL(tc.path, tc.version); got != want {
				t.Errorf("infoURL(%q, %q) = %q; want = %q", tc.path, tc.version, got, want)
			}

			want = fmt.Sprintf("%s/%s", client.url, tc.wantZipURL)
			if got, _ := client.zipURL(tc.path, tc.version); got != want {
				t.Errorf("zipURL(%q, %q) = %q; want = %q", tc.path, tc.version, got, want)
			}
		})
	}
}

func TestGetInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownProxy, client := SetupTestProxy(t, nil)
	defer teardownProxy(t)

	path := "my.mod/module"
	version := "v1.0.0"
	info, err := client.GetInfo(ctx, path, version)
	if err != nil {
		t.Fatalf("GetInfo(ctx, %q, %q) error: %v", path, version, err)
	}

	if info.Version != version {
		t.Errorf("VersionInfo.Version for GetInfo(ctx, %q, %q) = %q, want %q", path, version, info.Version, version)
	}

	expectedTime := time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
	if info.Time != expectedTime {
		t.Errorf("VersionInfo.Time for GetInfo(ctx, %q, %q) = %v, want %v", path, version, info.Time, expectedTime)
	}
}

func TestGetInfoVersionDoesNotExist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownProxy, client := SetupTestProxy(t, nil)
	defer teardownProxy(t)

	path := "my.mod/module"
	version := "v3.0.0"
	info, _ := client.GetInfo(ctx, path, version)
	if info != nil {
		t.Errorf("GetInfo(ctx, %q, %q) = %v, want %v", path, version, info, nil)
	}
}

func TestGetZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownProxy, client := SetupTestProxy(t, nil)
	defer teardownProxy(t)

	path := "my.mod/module"
	version := "v1.0.0"

	zipReader, err := client.GetZip(ctx, path, version)
	if err != nil {
		t.Fatalf("GetZip(ctx, %q, %q) error: %v", path, version, err)
	}

	expectedFiles := map[string]bool{
		"my.mod/module@v1.0.0/LICENSE":        true,
		"my.mod/module@v1.0.0/README.md":      true,
		"my.mod/module@v1.0.0/go.mod":         true,
		"my.mod/module@v1.0.0/foo/foo.go":     true,
		"my.mod/module@v1.0.0/foo/LICENSE.md": true,
		"my.mod/module@v1.0.0/bar/bar.go":     true,
		"my.mod/module@v1.0.0/bar/LICENSE":    true,
	}
	if len(zipReader.File) != len(expectedFiles) {
		t.Errorf("GetZip(ctx, %q, %q) returned number of files: got %d, want %d",
			path, version, len(zipReader.File), len(expectedFiles))
	}

	for _, zipFile := range zipReader.File {
		if !expectedFiles[zipFile.Name] {
			t.Errorf("GetZip(ctx, %q, %q) returned unexpected file: %q", path,
				version, zipFile.Name)
		}
		delete(expectedFiles, zipFile.Name)
	}
}

func TestGetZipNonExist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownProxy, client := SetupTestProxy(t, nil)
	defer teardownProxy(t)

	path := "my.mod/nonexistmodule"
	version := "v1.0.0"
	if _, err := client.zipURL(path, version); err != nil {
		t.Fatalf("client.zipURL(%q, %q): %v", path, version, err)
	}

	wantErrString := "Not Found"
	if _, err := client.GetZip(ctx, path, version); !strings.Contains(err.Error(), wantErrString) {
		t.Errorf("GetZip(ctx, %q, %q) returned error %v, want error containing %q",
			path, version, err, wantErrString)
	}
}

func TestEncodeModulePathAndVersion(t *testing.T) {
	for _, tc := range []struct {
		path, version, wantPath, wantVersion string
		wantErr                              bool
	}{
		{
			path:        "github.com/Azure/go-autorest",
			version:     "v11.0.0+incompatible",
			wantPath:    "github.com/!azure/go-autorest",
			wantVersion: "v11.0.0+incompatible",
			wantErr:     false,
		},
		{
			path:        "github.com/Azure/go-autorest",
			version:     "master",
			wantPath:    "github.com/!azure/go-autorest",
			wantVersion: "master",
			wantErr:     false,
		},
		{
			path:        "github.com/!azure/go-autorest",
			version:     "v11.0.0+incompatible",
			wantPath:    "github.com/!azure/go-autorest",
			wantVersion: "v11.0.0+incompatible",
			wantErr:     true,
		},
		{
			path:        "github.com/!azure/go-autorest",
			version:     "v11.0.0+incompatible",
			wantPath:    "github.com/!azure/go-autorest",
			wantVersion: "v11.0.0+incompatible",
			wantErr:     true,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			gotPath, gotVersion, err := encodeModulePathAndVersion(tc.path, tc.version)
			if err != nil && !tc.wantErr {
				t.Fatalf("encodeModulePathAndVersion(%q, %q): %v", tc.path, tc.version, err)
			}
			if err != nil && tc.wantErr {
				return
			}

			if gotPath != tc.wantPath || gotVersion != tc.wantVersion {
				t.Errorf("encodeModulePathAndVersion(%q, %q) = %q, %q, %v; want %q, %q, %v", tc.path, tc.version, gotPath, gotVersion, err, tc.wantPath, tc.wantVersion, nil)
			}
		})
	}
}
