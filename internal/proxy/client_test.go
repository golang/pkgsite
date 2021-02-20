// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

const testTimeout = 5 * time.Second

var testModule = &Module{
	ModulePath: sample.ModulePath,
	Version:    sample.VersionString,
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

const uncachedModulePath = "example.com/uncached"

var uncachedModule = &Module{
	ModulePath: uncachedModulePath,
	Version:    sample.VersionString,
	NotCached:  true,
}

func TestGetLatestInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testModules := []*Module{
		{
			ModulePath: sample.ModulePath,
			Version:    "v1.1.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.1"},
		},
		{
			ModulePath: sample.ModulePath,
			Version:    "v1.2.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.2"},
		},
	}
	client, teardownProxy := SetupTestClient(t, testModules)
	defer teardownProxy()

	info, err := client.GetInfo(ctx, sample.ModulePath, internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := info.Version, "v1.2.0"; got != want {
		t.Errorf("GetInfo(ctx, %q, %q): Version = %q, want %q", sample.ModulePath, internal.LatestVersion, got, want)
	}
}

func TestListVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testModules := []*Module{
		{
			ModulePath: sample.ModulePath,
			Version:    "v1.1.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.1"},
		},
		{
			ModulePath: sample.ModulePath,
			Version:    "v1.2.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.2"},
		},
		{
			ModulePath: sample.ModulePath + "/bar",
			Version:    "v1.3.0",
			Files:      map[string]string{"bar.go": "package bar\nconst Version = 1.3"},
		},
	}
	client, teardownProxy := SetupTestClient(t, testModules)
	defer teardownProxy()

	want := []string{"v1.1.0", "v1.2.0"}
	got, err := client.ListVersions(ctx, sample.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListVersions(%q) diff:\n%s", sample.ModulePath, diff)
	}
}

func TestGetInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestClient(t, []*Module{testModule, uncachedModule})
	defer teardownProxy()

	info, err := client.GetInfo(ctx, sample.ModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}

	if info.Version != sample.VersionString {
		t.Errorf("VersionInfo.Version for GetInfo(ctx, %q, %q) = %q, want %q",
			sample.ModulePath, sample.VersionString, info.Version, sample.VersionString)
	}
	expectedTime := time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
	if info.Time != expectedTime {
		t.Errorf("VersionInfo.Time for GetInfo(ctx, %q, %q) = %v, want %v", sample.ModulePath, sample.VersionString, info.Time, expectedTime)
	}

	// With fetch disabled, GetInfo returns "NotFetched" error on uncached module.
	noFetchClient := client.WithFetchDisabled()
	_, err = noFetchClient.GetInfo(ctx, uncachedModulePath, sample.VersionString)
	if !errors.Is(err, derrors.NotFetched) {
		t.Fatalf("got %v, want NotFetched", err)
	}
	// GetInfo with fetch disabled succeeds on a cached module.
	_, err = noFetchClient.GetInfo(ctx, sample.ModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetInfo_Errors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	proxyServer := NewServer(nil)
	proxyServer.AddRoute(
		fmt.Sprintf("/%s/@v/%s.info", "module.com/timeout", sample.VersionString),
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fetch timed out", http.StatusNotFound) })
	client, teardownProxy, err := NewClientForServer(proxyServer)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownProxy()

	for _, test := range []struct {
		modulePath string
		want       error
	}{
		{
			modulePath: sample.ModulePath,
			want:       derrors.NotFound,
		},
		{
			modulePath: "module.com/timeout",
			want:       derrors.ProxyTimedOut,
		},
	} {
		if _, err := client.GetInfo(ctx, test.modulePath, sample.VersionString); !errors.Is(err, test.want) {
			t.Errorf("GetInfo(ctx, %q, %q): %v, want %v", test.modulePath, sample.VersionString, err, test.want)
		}
	}
}

func TestGetMod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestClient(t, []*Module{testModule})
	defer teardownProxy()

	bytes, err := client.GetMod(ctx, sample.ModulePath, sample.VersionString)
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

	client, teardownProxy := SetupTestClient(t, []*Module{testModule})
	defer teardownProxy()

	zipReader, err := client.GetZip(ctx, sample.ModulePath, sample.VersionString)
	if err != nil {
		t.Fatal(err)
	}

	wantFiles := []string{
		sample.ModulePath + "@" + sample.VersionString + "/LICENSE",
		sample.ModulePath + "@" + sample.VersionString + "/README.md",
		sample.ModulePath + "@" + sample.VersionString + "/go.mod",
		sample.ModulePath + "@" + sample.VersionString + "/foo/foo.go",
		sample.ModulePath + "@" + sample.VersionString + "/foo/LICENSE.md",
		sample.ModulePath + "@" + sample.VersionString + "/bar/bar.go",
		sample.ModulePath + "@" + sample.VersionString + "/bar/LICENSE",
	}
	if len(zipReader.File) != len(wantFiles) {
		t.Errorf("GetZip(ctx, %q, %q) returned number of files: got %d, want %d",
			sample.ModulePath, sample.VersionString, len(zipReader.File), len(wantFiles))
	}

	expectedFileSet := map[string]bool{}
	for _, ef := range wantFiles {
		expectedFileSet[ef] = true
	}
	for _, zipFile := range zipReader.File {
		if !expectedFileSet[zipFile.Name] {
			t.Errorf("GetZip(ctx, %q, %q) returned unexpected file: %q", sample.ModulePath,
				sample.VersionString, zipFile.Name)
		}
		expectedFileSet[zipFile.Name] = false
	}
}

func TestGetZipNonExist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client, teardownProxy := SetupTestClient(t, nil)
	defer teardownProxy()

	if _, err := client.GetZip(ctx, sample.ModulePath, sample.VersionString); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want %v", err, derrors.NotFound)
	}
}

func TestGetZipSize(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		client, teardownProxy := SetupTestClient(t, []*Module{testModule})
		defer teardownProxy()
		got, err := client.GetZipSize(context.Background(), sample.ModulePath, sample.VersionString)
		if err != nil {
			t.Error(err)
		}
		const want = 3235
		if got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	})
	t.Run("not found", func(t *testing.T) {
		client, teardownProxy := SetupTestClient(t, nil)
		defer teardownProxy()
		if _, err := client.GetZipSize(context.Background(), sample.ModulePath, sample.VersionString); !errors.Is(err, derrors.NotFound) {
			t.Errorf("got %v, want %v", err, derrors.NotFound)
		}
	})
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
