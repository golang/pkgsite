// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

const testTimeout = 5 * time.Second

var sampleModule = &Module{
	ModulePath: "github.com/my/module",
	Version:    "v1.0.0",
	Files: map[string]string{
		"go.mod":      "module github.com/my/module\n\ngo 1.12",
		"LICENSE":     testhelper.BSD0License,
		"README.md":   "README FILE FOR TESTING.",
		"bar/LICENSE": testhelper.MITLicense,
		"bar/bar.go": `
						// package bar
						package bar

						// Bar returns the string "bar".
						func Bar() string {
							return "bar"
						}`,
		"foo/LICENSE.md": testhelper.MITLicense,
		"foo/foo.go": `
						// package foo
						package foo

						import (
							"fmt"

							"github.com/my/module/bar"
						)

						// FooBar returns the string "foo bar".
						func FooBar() string {
							return fmt.Sprintf("foo %s", bar.Bar())
						}`,
	},
}

func TestGetLatestInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	modulePath := "foo.com/bar"
	testModules := []*Module{
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.1.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.1"},
		},
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.2.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.2"},
		},
	}

	client, teardownProxy := SetupTestProxy(t, testModules)
	defer teardownProxy()

	info, err := client.GetInfo(ctx, modulePath, internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := info.Version, "v1.2.0"; got != want {
		t.Errorf("GetInfo(ctx, %q, %q): Version = %q, want %q", modulePath, internal.LatestVersion, got, want)
	}
}

func TestListVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	modulePath := "foo.com/bar"
	testModules := []*Module{
		{
			ModulePath: modulePath,
			Version:    "v1.1.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.1"},
		},
		{
			ModulePath: modulePath,
			Version:    "v1.2.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.2"},
		},
		{
			ModulePath: modulePath + "/bar",
			Version:    "v1.3.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.3"},
		},
	}

	client, teardownProxy := SetupTestProxy(t, testModules)
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

	client, teardownProxy := SetupTestProxy(t, []*Module{sampleModule})
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

	client, teardownProxy := SetupTestProxy(t, []*Module{sampleModule})
	defer teardownProxy()

	path := "github.com/my/module"
	version := "v3.0.0"
	info, _ := client.GetInfo(ctx, path, version)
	if info != nil {
		t.Errorf("GetInfo(ctx, %q, %q) = %v, want %v", path, version, info, nil)
	}
}

func TestGetMod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestProxy(t, []*Module{sampleModule})
	defer teardownProxy()

	path := "github.com/my/module"
	version := "v1.0.0"
	bytes, err := client.GetMod(ctx, path, version)
	if err != nil {
		t.Fatal(err)
	}
	got := string(bytes)
	want := "module github.com/my/module\n\ngo 1.12"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestProxy(t, []*Module{sampleModule})
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
	if _, err := client.GetZip(ctx, path, version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want %v", err, derrors.NotFound)
	}
}

func TestEncodedURL(t *testing.T) {
	c := &Client{url: "u"}
	for _, test := range []struct {
		path, version, suffix string
		want                  string // empty => error
	}{
		{
			"mod.com", "v1.0.0", "info",
			"u/mod.com/@v/v1.0.0.info",
		},
		{
			"mod", "v1.0.0", "info",
			"", // bad module path
		},
		{
			"mod.com", "v1.0.0-rc1", "info",
			"u/mod.com/@v/v1.0.0-rc1.info",
		},
		{
			"mod.com/Foo", "v1.0.0-RC1", "info",
			"u/mod.com/!foo/@v/v1.0.0-!r!c1.info",
		},
		{
			"mod.com", ".", "info",
			"", // bad version
		},
		{
			"mod.com", "v1.0.0", "zip",
			"u/mod.com/@v/v1.0.0.zip",
		},
		{
			"mod", "v1.0.0", "zip",
			"", // bad module path
		},
		{
			"mod.com", "v1.0.0-rc1", "zip",
			"u/mod.com/@v/v1.0.0-rc1.zip",
		},
		{
			"mod.com/Foo", "v1.0.0-RC1", "zip",
			"u/mod.com/!foo/@v/v1.0.0-!r!c1.zip",
		},
		{
			"mod.com", ".", "zip",
			"", // bad version
		},
		{
			"mod.com", internal.LatestVersion, "info",
			"u/mod.com/@latest",
		},
		{
			"mod.com", internal.LatestVersion, "zip",
			"", // can't ask for latest zip
		},
		{
			"mod.com", "v1.0.0", "other",
			"", // only "info" or "zip"
		},
	} {
		got, err := c.escapedURL(test.path, test.version, test.suffix)
		if got != test.want || (err != nil) != (test.want == "") {
			t.Errorf("%s, %s, %s: got (%q, %v), want %q", test.path, test.version, test.suffix, got, err, test.want)
		}
	}
}
