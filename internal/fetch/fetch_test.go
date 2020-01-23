// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/discovery/internal/testing/testhelper"
	"golang.org/x/discovery/internal/version"
)

const testTimeout = 30 * time.Second

func TestExtractPackagesFromZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name                 string
		version              string
		packages             map[string]*internal.Package
		packageVersionStates []*internal.PackageVersionState
	}{
		{
			name:    "github.com/my/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"bar": {
					Name:              "bar",
					Path:              "github.com/my/module/bar",
					Synopsis:          "package bar",
					DocumentationHTML: "Bar returns the string &#34;bar&#34;.",
					Imports:           []string{},
					V1Path:            "github.com/my/module/bar",
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
				"foo": {
					Name:              "foo",
					Path:              "github.com/my/module/foo",
					Synopsis:          "package foo",
					DocumentationHTML: "FooBar returns the string &#34;foo bar&#34;.",
					Imports:           []string{"fmt", "github.com/my/module/bar"},
					V1Path:            "github.com/my/module/foo",
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		},
		{
			name:    "no.mod/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"p": {
					Name:              "p",
					Path:              "no.mod/module/p",
					Synopsis:          "Package p is inside a module where a go.mod file hasn't been explicitly added yet.",
					DocumentationHTML: "const Year = 2009",
					Imports:           []string{},
					V1Path:            "no.mod/module/p",
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
		},
		{
			name:     "emp.ty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
		{
			name:    "emp.ty/package",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"main": {
					Name:     "main",
					Path:     "emp.ty/package",
					Synopsis: "",
					Imports:  []string{},
					V1Path:   "emp.ty/package",
					GOOS:     "linux",
					GOARCH:   "amd64",
				},
			},
		},
		{
			name:    "bad.mod/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"good": {
					Name:              "good",
					Path:              "bad.mod/module/good",
					Synopsis:          "Package good is inside a module that has bad packages.",
					DocumentationHTML: `const Good = <a href="/pkg/builtin#true">true</a>`,
					Imports:           []string{},
					V1Path:            "bad.mod/module/good",
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
			packageVersionStates: []*internal.PackageVersionState{
				{
					PackagePath: "bad.mod/module/good",
					ModulePath:  "bad.mod/module",
					Version:     "v1.0.0",
					Status:      200,
				},
				{
					PackagePath: "bad.mod/module/illegalchar",
					ModulePath:  "bad.mod/module",
					Version:     "v1.0.0",
					Status:      600,
				},
				{
					PackagePath: "bad.mod/module/multiplepkgs",
					ModulePath:  "bad.mod/module",
					Version:     "v1.0.0",
					Status:      600,
				},
			},
		},
		{
			name:    "build.constraints/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"cpu": {
					Name:              "cpu",
					Path:              "build.constraints/module/cpu",
					Synopsis:          "Package cpu implements processor feature detection used by the Go standard library.",
					DocumentationHTML: "const CacheLinePadSize = 3",
					Imports:           []string{},
					V1Path:            "build.constraints/module/cpu",
					GOOS:              "linux",
					GOARCH:            "amd64",
				},
			},
			packageVersionStates: []*internal.PackageVersionState{
				{
					ModulePath:  "build.constraints/module",
					Version:     "v1.0.0",
					PackagePath: "build.constraints/module/cpu",
					Status:      http.StatusOK,
				},
				{
					ModulePath:  "build.constraints/module",
					Version:     "v1.0.0",
					PackagePath: "build.constraints/module/ignore",
					Status:      derrors.ToHTTPStatus(derrors.BuildContextNotSupported),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, teardownProxy := proxy.SetupTestProxy(t, nil)
			defer teardownProxy()

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatal(err)
			}

			packages, pvstates, err := extractPackagesFromZip(context.Background(), test.name, test.version, reader, nil, nil)
			if err != nil && len(test.packages) != 0 {
				t.Fatalf("extractPackagesFromZip(%q, %q, reader, nil): %v", test.name, test.version, err)
			}

			if test.packageVersionStates == nil {
				for _, p := range test.packages {
					test.packageVersionStates = append(test.packageVersionStates,
						&internal.PackageVersionState{
							ModulePath:  test.name,
							Version:     test.version,
							PackagePath: p.Path,
							Status:      http.StatusOK,
						})
				}
				sort.Slice(test.packageVersionStates, func(i, j int) bool {
					return test.packageVersionStates[i].PackagePath < test.packageVersionStates[j].PackagePath
				})
			}
			sort.Slice(pvstates, func(i, j int) bool {
				return pvstates[i].PackagePath < pvstates[j].PackagePath
			})
			if diff := cmp.Diff(test.packageVersionStates, pvstates, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(internal.PackageVersionState{}, "Error")); diff != "" {
				t.Fatalf("extractPackagesFromZip(%q, %q, reader, nil) mismatch for packageVersionStates (-want +got):\n%s", test.name, test.version, diff)
			}

			for _, got := range packages {
				want, ok := test.packages[got.Name]
				if !ok {
					t.Errorf("extractPackagesFromZip(%q, %q, reader, nil) returned unexpected package: %q", test.name, test.version, got.Name)
					continue
				}

				sort.Strings(got.Imports)

				if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML")); diff != "" {
					t.Errorf("extractPackagesFromZip(%q, %q, reader, nil) mismatch (-want +got):\n%s", test.name, test.version, diff)
				}

				if got, want := got.DocumentationHTML, want.DocumentationHTML; len(want) == 0 && len(got) != 0 {
					t.Errorf("got non-empty documentation but want empty:\ngot: %q\nwant: %q", got, want)
				} else if !strings.Contains(got, want) {
					t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
				}
			}
		})
	}
}

func TestExtractReadmeFromZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name, version, file, wantPath string
		wantContents                  string
		err                           error
	}{
		{
			name:         "github.com/my/module",
			version:      "v1.0.0",
			file:         "github.com/my/module@v1.0.0/README.md",
			wantPath:     "README.md",
			wantContents: "README FILE FOR TESTING.",
		},
		{
			name:    "emp.ty/module",
			version: "v1.0.0",
			err:     errReadmeNotFound,
		},
	} {
		t.Run(test.file, func(t *testing.T) {
			client, teardownProxy := proxy.SetupTestProxy(t, nil)
			defer teardownProxy()

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatal(err)
			}

			gotPath, gotContents, err := extractReadmeFromZip(test.name, test.version, reader)
			if err != nil {
				if test.err == nil || test.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						fmt.Sprintf("%q %q", test.name, test.version), filepath.Base(test.file), err, test.err)
				} else {
					return
				}
			}

			if test.wantPath != gotPath {
				t.Errorf("extractFile(%q, %q) path = %q, want %q", test.name, test.file, gotPath, test.wantPath)
			}
			if test.wantContents != gotContents {
				t.Errorf("extractFile(%q, %q) contents = %q, want %q", test.name, test.file, gotContents, test.wantContents)
			}
		})
	}
}

func TestHasFilename(t *testing.T) {
	for _, test := range []struct {
		file         string
		expectedFile string
		want         bool
	}{
		{
			file:         "github.com/my/module@v1.0.0/README.md",
			expectedFile: "README.md",
			want:         true,
		},
		{
			file:         "rEaDme",
			expectedFile: "README",
			want:         true,
		}, {
			file:         "README.FOO",
			expectedFile: "README",
			want:         true,
		},
		{
			file:         "FOO_README",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README_FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README.FOO.FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "github.com/my/module@v1.0.0/LICENSE",
			expectedFile: "github.com/my/module@v1.0.0/LICENSE",
			want:         true,
		},
	} {
		{
			t.Run(test.file, func(t *testing.T) {
				got := hasFilename(test.file, test.expectedFile)
				if got != test.want {
					t.Errorf("hasFilename(%q, %q) = %t: %t", test.file, test.expectedFile, got, test.want)
				}
			})
		}
	}
}

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

func TestFetchVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	modulePath := "github.com/my/module"
	vers := "v1.0.0"
	wantVersionInfo := internal.VersionInfo{
		ModulePath:        "github.com/my/module",
		Version:           "v1.0.0",
		CommitTime:        testProxyCommitTime,
		ReadmeFilePath:    "README.md",
		ReadmeContents:    "THIS IS A README",
		VersionType:       version.TypeRelease,
		IsRedistributable: true,
		SourceInfo:        source.NewGitHubInfo("https://github.com/my/module", "", "v1.0.0"),
	}
	wantCoverage := sample.LicenseMetadata[0].Coverage
	wantLicenses := []*licenses.License{
		{
			Metadata: &licenses.Metadata{
				Types:    []string{"MIT"},
				FilePath: "LICENSE.md",
				Coverage: wantCoverage,
			},
			Contents: []byte(testhelper.MITLicense),
		},
	}
	for _, test := range []struct {
		name     string
		contents map[string]string
		want     *internal.Version
	}{
		{
			name: "basic",
			contents: map[string]string{
				"README.md":  "THIS IS A README",
				"foo/foo.go": "// package foo exports a helpful constant.\npackage foo\nimport \"net/http\"\nconst OK = http.StatusOK",
				"LICENSE.md": testhelper.MITLicense,
			},
			want: &internal.Version{
				VersionInfo: wantVersionInfo,
				Packages: []*internal.Package{
					{
						Path:              "github.com/my/module/foo",
						V1Path:            "github.com/my/module/foo",
						Name:              "foo",
						Synopsis:          "package foo exports a helpful constant.",
						IsRedistributable: true,
						Licenses: []*licenses.Metadata{
							{Types: []string{"MIT"}, FilePath: "LICENSE.md", Coverage: wantCoverage},
						},
						Imports: []string{"net/http"},
						GOOS:    "linux",
						GOARCH:  "amd64",
					},
				},
				Licenses: wantLicenses,
			},
		},
		{
			name: "wasm",
			contents: map[string]string{
				"README.md":  "THIS IS A README",
				"LICENSE.md": testhelper.MITLicense,
				"js/js.go": `
					// +build js,wasm

					// Package js only works with wasm.
					package js
					type Value int`,
			},
			want: &internal.Version{
				VersionInfo: wantVersionInfo,
				Packages: []*internal.Package{
					{
						Path:              "github.com/my/module/js",
						V1Path:            "github.com/my/module/js",
						Name:              "js",
						Synopsis:          "Package js only works with wasm.",
						IsRedistributable: true,
						Licenses: []*licenses.Metadata{
							{Types: []string{"MIT"}, FilePath: "LICENSE.md", Coverage: wantCoverage},
						},
						Imports: []string{},
						GOOS:    "js",
						GOARCH:  "wasm",
					},
				},
				Licenses: wantLicenses,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestVersion{
				proxy.NewTestVersion(t, modulePath, vers, test.contents),
			})
			defer teardownProxy()

			got, err := FetchVersion(ctx, modulePath, vers, client)
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"),
				cmp.AllowUnexported(source.Info{}),
			}
			opts = append(opts, sample.LicenseCmpOpts...)
			if diff := cmp.Diff(test.want, got.Version, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if got.GoModPath != modulePath {
				t.Errorf("go.mod path: got %q, want %q", got.GoModPath, modulePath)
			}
		})
	}
}

func TestFetchVersion_Alternative(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const (
		modulePath = "github.com/my/module"
		goModPath  = "canonical"
		vers       = "v1.0.0"
	)

	client, teardownProxy := proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, modulePath, vers, map[string]string{"go.mod": "module " + goModPath}),
	})
	defer teardownProxy()

	res, err := FetchVersion(ctx, modulePath, vers, client)
	if !errors.Is(err, derrors.AlternativeModule) {
		t.Errorf("got %v, want derrors.AlternativeModule", err)
	}
	if res == nil || res.GoModPath != goModPath {
		t.Errorf("got %+v, wanted GoModPath %q", res, goModPath)
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
