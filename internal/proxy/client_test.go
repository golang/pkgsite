// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/xerrors"
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

func TestModulePathAndVersionForProxyRequest(t *testing.T) {
	for _, tc := range []struct {
		name, requestedPath, requestedVersion, wantPath, wantVersion string
	}{
		{
			name:             "module with tagged version",
			requestedPath:    "google.golang.org/api",
			requestedVersion: "v1.0.0",
			wantPath:         "google.golang.org/api",
			wantVersion:      "v1.0.0",
		},
		{
			name:             "must encode path and version",
			requestedPath:    "github.com/Azure/azure-sdk-for-go",
			requestedVersion: "v8.0.1-beta+incompatible",
			wantPath:         "github.com/!azure/azure-sdk-for-go",
			wantVersion:      "v8.0.1-beta+incompatible",
		},
		{
			name:             "std version v1.12.5",
			requestedPath:    "std",
			requestedVersion: "v1.12.5",
			wantPath:         stdlibProxyModulePathPrefix,
			wantVersion:      "go1.12.5",
		},
		{
			name:             "std version v1.13, incomplete canonical version",
			requestedPath:    "std",
			requestedVersion: "v1.13",
			wantPath:         stdlibProxyModulePathPrefix + "/src",
			wantVersion:      "go1.13",
		},
		{
			name:             "std version v1.13.0-beta.1",
			requestedPath:    "std",
			requestedVersion: "v1.13.0-beta.1",
			wantPath:         stdlibProxyModulePathPrefix + "/src",
			wantVersion:      "go1.13beta1",
		},
		{
			name:             "std with digitless prerelease",
			requestedPath:    "std",
			requestedVersion: "v1.13.0-prerelease",
			wantPath:         stdlibProxyModulePathPrefix + "/src",
			wantVersion:      "go1.13prerelease",
		},
		{
			name:             "cmd version v1.13.0",
			requestedPath:    "cmd",
			requestedVersion: "v1.13.0",
			wantPath:         stdlibProxyModulePathPrefix + "/src/cmd",
			wantVersion:      "go1.13",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, version, err := modulePathAndVersionForProxyRequest(tc.requestedPath, tc.requestedVersion)
			if err != nil {
				t.Fatalf("modulePathAndVersionForProxyRequest(%q, %q): %v", tc.requestedPath, tc.requestedVersion, err)
			}

			if path != tc.wantPath || version != tc.wantVersion {
				t.Errorf("modulePathAndVersionForProxyRequest(%q, %q) = %q, %q; want = %q, %q", tc.requestedPath, tc.requestedVersion, path, version, tc.wantPath, tc.wantVersion)
			}
		})
	}
}

func TestModulePathAndVersionForProxyRequestErrors(t *testing.T) {
	for _, tc := range []struct {
		name, requestedPath, requestedVersion string
	}{
		{
			name:             "bad std semver",
			requestedPath:    "std",
			requestedVersion: "v1.x",
		},
		{
			name:             "more bad std semver",
			requestedPath:    "std",
			requestedVersion: "v1.0-",
		},
		{
			name:             "bad std cmd version",
			requestedPath:    "cmd",
			requestedVersion: "v1.12",
		},
		{
			name:             "bad prerelease",
			requestedPath:    "std",
			requestedVersion: "v1.13.0-beta1",
		},
		{
			name:             "another bad prerelease",
			requestedPath:    "std",
			requestedVersion: "v1.13.0-whatevs99",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := modulePathAndVersionForProxyRequest(tc.requestedPath, tc.requestedVersion)
			if !xerrors.Is(err, derrors.InvalidArgument) {
				t.Fatalf("modulePathAndVersionForProxyRequest(%q, %q): got %v, want InvalidArgument error",
					tc.requestedPath, tc.requestedVersion, err)
			}
		})
	}
}

func TestGetLatestInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	modulePath := "foo.com/bar"
	testVersions := []*TestVersion{
		NewTestVersion(t, "foo.com/bar", "v1.1.0", map[string]string{"bar.go": "package bar\nconst Version = 1.1"}),
		NewTestVersion(t, "foo.com/bar", "v1.2.0", map[string]string{"bar.go": "package bar\nconst Version = 1.2"}),
	}

	client, teardownProxy := SetupTestProxy(t, testVersions)
	defer teardownProxy()

	info, err := client.GetLatestInfo(ctx, modulePath)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := info.Version, "v1.2.0"; got != want {
		t.Errorf("GetLatestInfo(ctx, %q): Version = %q, want %q", modulePath, got, want)
	}
}

func TestListVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	modulePath := "foo.com/bar"
	testVersions := []*TestVersion{
		NewTestVersion(t, "foo.com/bar", "v1.1.0", map[string]string{"bar.go": "package bar\nconst Version = 1.1"}),
		NewTestVersion(t, "foo.com/bar", "v1.2.0", map[string]string{"bar.go": "package bar\nconst Version = 1.2"}),
		NewTestVersion(t, "foo.com/baz", "v1.3.0", map[string]string{"baz.go": "package bar\nconst Version = 1.3"}),
	}

	client, teardownProxy := SetupTestProxy(t, testVersions)
	defer teardownProxy()

	want := []string{"v1.1.0", "v1.2.0"}
	got, err := client.ListVersions(ctx, modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListVersions(%q) diff:\n%s", modulePath, diff)
	}
}

func TestGetInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestProxy(t, nil)
	defer teardownProxy()

	path := "github.com/my/module"
	version := "v1.0.0"
	info, err := client.GetInfo(ctx, path, version)
	if err != nil {
		t.Fatal(err)
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

	client, teardownProxy := SetupTestProxy(t, nil)
	defer teardownProxy()

	path := "github.com/my/module"
	version := "v3.0.0"
	info, _ := client.GetInfo(ctx, path, version)
	if info != nil {
		t.Errorf("GetInfo(ctx, %q, %q) = %v, want %v", path, version, info, nil)
	}
}

func TestGetZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestProxy(t, nil)
	defer teardownProxy()

	for _, tc := range []struct {
		path, version string
		wantFiles     []string
	}{
		{
			path:    "github.com/my/module",
			version: "v1.0.0",
			wantFiles: []string{
				"github.com/my/module@v1.0.0/LICENSE",
				"github.com/my/module@v1.0.0/README.md",
				"github.com/my/module@v1.0.0/go.mod",
				"github.com/my/module@v1.0.0/foo/foo.go",
				"github.com/my/module@v1.0.0/foo/LICENSE.md",
				"github.com/my/module@v1.0.0/bar/bar.go",
				"github.com/my/module@v1.0.0/bar/LICENSE",
			},
		},
		{
			path:    "std",
			version: "v1.12.5",
			wantFiles: []string{
				"std@v1.12.5/LICENSE",
				"std@v1.12.5/builtin/builtin.go",
				"std@v1.12.5/context/benchmark_test.go",
				"std@v1.12.5/context/context.go",
				"std@v1.12.5/context/context_test.go",
				"std@v1.12.5/context/example_test.go",
				"std@v1.12.5/context/net_test.go",
				"std@v1.12.5/context/x_test.go",
			},
		},
		{
			path:    "cmd",
			version: "v1.13.0-beta.1",
			wantFiles: []string{
				"cmd@v1.13.0-beta.1/LICENSE",
				"cmd@v1.13.0-beta.1/go/go11.go",
			},
		},
		{
			path:    "std",
			version: "v1.13.0-beta.1",
			wantFiles: []string{
				"std@v1.13.0-beta.1/LICENSE",
				"std@v1.13.0-beta.1/context/benchmark_test.go",
				"std@v1.13.0-beta.1/context/context.go",
				"std@v1.13.0-beta.1/context/context_test.go",
				"std@v1.13.0-beta.1/context/example_test.go",
				"std@v1.13.0-beta.1/context/net_test.go",
				"std@v1.13.0-beta.1/context/x_test.go",
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			zipReader, err := client.GetZip(ctx, tc.path, tc.version)
			if err != nil {
				t.Fatal(err)
			}

			if len(zipReader.File) != len(tc.wantFiles) {
				t.Errorf("GetZip(ctx, %q, %q) returned number of files: got %d, want %d",
					tc.path, tc.version, len(zipReader.File), len(tc.wantFiles))
			}

			expectedFileSet := map[string]bool{}
			for _, ef := range tc.wantFiles {
				expectedFileSet[ef] = true
			}
			for _, zipFile := range zipReader.File {
				if !expectedFileSet[zipFile.Name] {
					t.Errorf("GetZip(ctx, %q, %q) returned unexpected file: %q", tc.path,
						tc.version, zipFile.Name)
				}
				expectedFileSet[zipFile.Name] = false
			}
		})
	}
}

func TestGetZipNonExist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestProxy(t, nil)
	defer teardownProxy()

	path := "my.mod/nonexistmodule"
	version := "v1.0.0"
	if _, err := client.GetZip(ctx, path, version); !xerrors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want %v", err, derrors.NotFound)
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
