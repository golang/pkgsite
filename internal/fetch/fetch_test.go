// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/discovery/internal/testing/testhelper"
	"golang.org/x/discovery/internal/version"
)

var (
	testTimeout         = 30 * time.Second
	sourceTimeout       = 1 * time.Second
	testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
	defaultLicenses     = []*licenses.License{
		{
			Metadata: &licenses.Metadata{
				Types:    []string{"BSD-0-Clause"},
				FilePath: "LICENSE",
				Coverage: licensecheck.Coverage{
					Percent: 100,
					Match: []licensecheck.Match{
						{
							Name:    "BSD-0-Clause",
							Type:    licensecheck.BSD,
							Percent: 100,
						},
					},
				},
			},
			Contents: []byte(testhelper.BSD0License),
		},
	}
)

func TestFetchVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	setPackageVersionStates := func(fr *FetchResult) {
		if fr.PackageVersionStates != nil {
			return
		}
		for _, p := range fr.Module.Packages {
			fr.PackageVersionStates = append(
				fr.PackageVersionStates, &internal.PackageVersionState{
					PackagePath: p.Path,
					ModulePath:  fr.Module.ModulePath,
					Version:     "v1.0.0",
					Status:      http.StatusOK,
				},
			)
		}
	}
	setFetchResult := func(modulePath string, fr *FetchResult) {
		if fr.GoModPath == "" {
			fr.GoModPath = modulePath
		}
		if fr.Module.Version == "" {
			fr.Module.Version = "v1.0.0"
		}
		if fr.Module.VersionType == "" {
			fr.Module.VersionType = version.TypeRelease
		}
		if fr.Module.CommitTime.IsZero() {
			fr.Module.CommitTime = testProxyCommitTime
		}
		if fr.Module.Licenses == nil {
			fr.Module.Licenses = defaultLicenses
			for _, p := range fr.Module.Packages {
				p.Licenses = []*licenses.Metadata{defaultLicenses[0].Metadata}
			}
		}
	}

	sortFetchResult := func(fr *FetchResult) {
		sort.Slice(fr.Module.Packages, func(i, j int) bool {
			return fr.Module.Packages[i].Path < fr.Module.Packages[j].Path
		})
		sort.Slice(fr.Module.Licenses, func(i, j int) bool {
			return fr.Module.Licenses[i].FilePath < fr.Module.Licenses[j].FilePath
		})
		sort.Slice(fr.PackageVersionStates, func(i, j int) bool {
			return fr.PackageVersionStates[i].PackagePath < fr.PackageVersionStates[j].PackagePath
		})
	}

	checkDocumentationHTML := func(fr *FetchResult, got *FetchResult) {
		for i := 0; i < len(fr.Module.Packages); i++ {
			want := fr.Module.Packages[i].DocumentationHTML
			got := got.Module.Packages[i].DocumentationHTML
			if len(want) != 0 && !strings.Contains(got, want) {
				t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
			}
		}
	}

	sourceClient := source.NewClient(sourceTimeout)
	for _, test := range []struct {
		name string
		mod  *testModule
	}{
		{name: "basic", mod: moduleNoGoMod},
		{name: "wasm", mod: moduleWasm},
		{name: "no go.mod file", mod: moduleOnePackage},
		{name: "has go.mod", mod: moduleMultiPackage},
		{name: "module with bad packages", mod: moduleBadPackages},
		{name: "module with build constraints", mod: moduleBuildConstraints},
		{name: "module with packages with bad import paths", mod: moduleBadImportPath},
		{name: "module with documentation", mod: moduleDocTest},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{{
				ModulePath: test.mod.mod.ModulePath,
				Files:      test.mod.mod.Files,
			}})
			defer teardownProxy()

			got, err := FetchVersion(ctx, test.mod.mod.ModulePath, "v1.0.0", proxyClient, sourceClient)
			if err != nil {
				t.Fatal(err)
			}
			setFetchResult(test.mod.mod.ModulePath, test.mod.fr)
			setPackageVersionStates(test.mod.fr)
			opts := []cmp.Option{
				cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"),
				cmpopts.IgnoreFields(internal.PackageVersionState{}, "Error"),
				cmp.AllowUnexported(source.Info{}),
			}
			opts = append(opts, sample.LicenseCmpOpts...)
			sortFetchResult(test.mod.fr)
			sortFetchResult(got)
			checkDocumentationHTML(test.mod.fr, got)
			if diff := cmp.Diff(test.mod.fr, got, opts...); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
func TestFetchVersion_Errors(t *testing.T) {
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
			proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{{
				ModulePath: test.mod.mod.ModulePath,
				Files:      test.mod.mod.Files,
			}})
			defer teardownProxy()

			sourceClient := source.NewClient(sourceTimeout)
			got, err := FetchVersion(ctx, test.mod.mod.ModulePath, "v1.0.0", proxyClient, sourceClient)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("FetchVersion(ctx, %q, v1.0.0, proxyClient, sourceClient): %v; wantErr = %v)", test.mod.mod.ModulePath, err, test.wantErr)
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
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	proxyClient, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestModule{
		{
			ModulePath: "github.com/my/module",
			Files: map[string]string{
				"README.md": "README FILE FOR TESTING.",
			},
		},
		{
			ModulePath: "emp.ty/module",
			Files:      map[string]string{},
		},
	})
	defer teardownProxy()

	for _, test := range []struct {
		modulePath, wantPath string
		wantContents         []*internal.Readme
	}{
		{
			modulePath: "github.com/my/module",
			wantPath:   "README.md",
			wantContents: []*internal.Readme{
				{
					Filepath: "README.md",
					Contents: "README FILE FOR TESTING.",
				},
			},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			reader, err := proxyClient.GetZip(ctx, test.modulePath, "v1.0.0")
			if err != nil {
				t.Fatal(err)
			}

			gotContents, err := extractReadmesFromZip(test.modulePath, "v1.0.0", reader)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantContents, gotContents); diff != "" {
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
			file: "Readme.go",
			name: ".go README file will return false",
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
