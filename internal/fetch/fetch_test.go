// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var (
	testTimeout   = 30 * time.Second
	sourceTimeout = 1 * time.Second
)

func TestFetchModule(t *testing.T) {
	stdlib.UseTestData = true

	// Stub out the function used to share playground snippets
	origPost := httpPost
	httpPost = func(url string, contentType string, body io.Reader) (resp *http.Response, err error) {
		w := httptest.NewRecorder()
		w.WriteHeader(http.StatusOK)
		return w.Result(), nil
	}
	defer func() { httpPost = origPost }()

	defer func(oldmax int) { MaxDocumentationHTML = oldmax }(MaxDocumentationHTML)
	MaxDocumentationHTML = 1 * megabyte

	for _, test := range []struct {
		name         string
		mod          *testModule
		fetchVersion string
	}{
		{name: "basic", mod: moduleNoGoMod},
		{name: "wasm", mod: moduleWasm},
		{name: "no go.mod file", mod: moduleOnePackage},
		{name: "has go.mod", mod: moduleMultiPackage},
		{name: "module with bad packages", mod: moduleBadPackages},
		{name: "module with build constraints", mod: moduleBuildConstraints},
		{name: "module with packages with bad import paths", mod: moduleBadImportPath},
		{name: "module with documentation", mod: moduleDocTest},
		{name: "documentation too large", mod: moduleDocTooLarge},
		{name: "module with package-level example", mod: modulePackageExample},
		{name: "module with function example", mod: moduleFuncExample},
		{name: "module with type example", mod: moduleTypeExample},
		{name: "module with method example", mod: moduleMethodExample},
		{name: "module with nonredistributable packages", mod: moduleNonRedist},
		{name: "stdlib module", mod: moduleStd},
		{name: "master version of module", mod: moduleMaster, fetchVersion: "master"},
		{name: "latest version of module", mod: moduleLatest, fetchVersion: "latest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := experiment.NewContext(context.Background(), internal.ExperimentExecutableExamples)
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			modulePath := test.mod.mod.ModulePath
			version := test.mod.mod.Version
			fetchVersion := test.fetchVersion
			if version == "" {
				version = "v1.0.0"
			}
			if fetchVersion == "" {
				fetchVersion = version
			}
			sourceClient := source.NewClient(sourceTimeout)
			proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{{
				ModulePath: modulePath,
				Version:    version,
				Files:      test.mod.mod.Files,
			}})
			defer teardownProxy()
			got := FetchModule(ctx, modulePath, fetchVersion, proxyClient, sourceClient)
			if got.Error != nil {
				t.Fatal(got.Error)
			}
			d := licenseDetector(ctx, t, modulePath, fetchVersion, proxyClient)
			fr := cleanFetchResult(test.mod.fr, d)
			sortFetchResult(fr)
			sortFetchResult(got)
			opts := []cmp.Option{
				cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"),
				cmpopts.IgnoreFields(internal.Documentation{}, "HTML"),
				cmpopts.IgnoreFields(internal.PackageVersionState{}, "Error"),
				cmp.AllowUnexported(source.Info{}),
				cmpopts.EquateEmpty(),
			}
			opts = append(opts, sample.LicenseCmpOpts...)
			if diff := cmp.Diff(fr, got, opts...); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
			validateDocumentationHTML(t, got.Module, fr.Module)
		})
	}
}
func TestFetchModule_Errors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	for _, test := range []struct {
		name          string
		mod           *testModule
		wantErr       error
		wantGoModPath string
	}{
		{name: "alternative", mod: moduleAlternative, wantErr: derrors.AlternativeModule, wantGoModPath: "canonical"},
		{name: "empty module", mod: moduleEmpty, wantErr: derrors.BadModule},
	} {
		t.Run(test.name, func(t *testing.T) {
			modulePath := test.mod.mod.ModulePath
			version := test.mod.mod.Version
			if version == "" {
				version = "v1.0.0"
			}
			proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{{
				ModulePath: modulePath,
				Files:      test.mod.mod.Files,
			}})
			defer teardownProxy()

			sourceClient := source.NewClient(sourceTimeout)
			got := FetchModule(ctx, modulePath, "v1.0.0", proxyClient, sourceClient)
			if !errors.Is(got.Error, test.wantErr) {
				t.Fatalf("FetchModule(ctx, %q, v1.0.0, proxyClient, sourceClient): %v; wantErr = %v)", modulePath, got.Error, test.wantErr)
			}
			if test.wantGoModPath != "" {
				if got == nil || got.GoModPath != test.wantGoModPath {
					t.Errorf("got %+v, wanted GoModPath %q", got, test.wantGoModPath)
				}
			}
		})
	}
}

func TestExtractReadmesFromZip(t *testing.T) {
	stdlib.UseTestData = true

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	sortReadmes := func(readmes []*internal.Readme) {
		sort.Slice(readmes, func(i, j int) bool {
			return readmes[i].Filepath < readmes[j].Filepath
		})
	}

	for _, test := range []struct {
		modulePath, version string
		files               map[string]string
		want                []*internal.Readme
	}{
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.12.5",
			want: []*internal.Readme{
				{
					Filepath: "README.md",
					Contents: "# The Go Programming Language\n",
				},
				{
					Filepath: "cmd/pprof/README",
					Contents: "This directory is the copy of Google's pprof shipped as part of the Go distribution.\n",
				},
			},
		},
		{
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			files: map[string]string{
				"README.md":  "README FILE FOR TESTING.",
				"foo/README": "Another README",
			},
			want: []*internal.Readme{
				{
					Filepath: "README.md",
					Contents: "README FILE FOR TESTING.",
				},
				{
					Filepath: "foo/README",
					Contents: "Another README",
				},
			},
		},
		{
			modulePath: "emp.ty/module",
			version:    "v1.0.0",
			files:      map[string]string{},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			var (
				reader *zip.Reader
				err    error
			)
			if test.modulePath == stdlib.ModulePath {
				reader, _, _, err = stdlib.Zip(test.version)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
					{ModulePath: test.modulePath, Files: test.files}})
				defer teardownProxy()
				reader, err = proxyClient.GetZip(ctx, test.modulePath, "v1.0.0")
				if err != nil {
					t.Fatal(err)
				}
			}

			got, err := extractReadmesFromZip(test.modulePath, test.version, reader)
			if err != nil {
				t.Fatal(err)
			}

			sortReadmes(test.want)
			sortReadmes(got)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsReadme(t *testing.T) {
	for _, test := range []struct {
		name, file string
		want       bool
	}{
		{
			name: "README in nested dir returns true",
			file: "github.com/my/module@v1.0.0/README.md",
			want: true,
		},
		{
			name: "case insensitive",
			file: "rEaDme",
			want: true,
		},
		{
			name: "random extension returns true",
			file: "README.FOO",
			want: true,
		},
		{
			name: "{prefix}readme will return false",
			file: "FOO_README",
			want: false,
		},
		{
			file: "README_FOO",
			name: "readme{suffix} will return false",
			want: false,
		},
		{
			file: "README.FOO.FOO",
			name: "README file with multiple extensions will return false",
			want: false,
		},
		{
			file: "readme.go",
			name: ".go README file will return false",
			want: false,
		},
		{
			file: "readme.vendor",
			name: ".vendor README file will return false",
			want: false,
		},
		{
			file: "",
			name: "empty filename returns false",
			want: false,
		},
	} {
		{
			t.Run(test.file, func(t *testing.T) {
				if got := isReadme(test.file); got != test.want {
					t.Errorf("isReadme(%q) = %t: %t", test.file, got, test.want)
				}
			})
		}
	}
}

func TestMatchingFiles(t *testing.T) {
	plainGoBody := `
		package plain
		type Value int`
	jsGoBody := `
		// +build js,wasm

		// Package js only works with wasm.
		package js
		type Value int`

	plainContents := map[string]string{
		"README.md":      "THIS IS A README",
		"LICENSE.md":     testhelper.MITLicense,
		"plain/plain.go": plainGoBody,
	}

	jsContents := map[string]string{
		"README.md":  "THIS IS A README",
		"LICENSE.md": testhelper.MITLicense,
		"js/js.go":   jsGoBody,
	}
	for _, test := range []struct {
		name         string
		goos, goarch string
		contents     map[string]string
		want         map[string][]byte
	}{
		{
			name:     "plain-linux",
			goos:     "linux",
			goarch:   "amd64",
			contents: plainContents,
			want: map[string][]byte{
				"plain.go": []byte(plainGoBody),
			},
		},
		{
			name:     "plain-js",
			goos:     "js",
			goarch:   "wasm",
			contents: plainContents,
			want: map[string][]byte{
				"plain.go": []byte(plainGoBody),
			},
		},
		{
			name:     "wasm-linux",
			goos:     "linux",
			goarch:   "amd64",
			contents: jsContents,
			want:     map[string][]byte{},
		},
		{
			name:     "wasm-js",
			goos:     "js",
			goarch:   "wasm",
			contents: jsContents,
			want: map[string][]byte{
				"js.go": []byte(jsGoBody),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := testhelper.ZipContents(test.contents)
			if err != nil {
				t.Fatal(err)
			}
			r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := matchingFiles(test.goos, test.goarch, r.File)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
