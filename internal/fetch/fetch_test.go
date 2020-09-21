// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
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
			ctx := context.Background()
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
			defer got.Defer()
			if got.Error != nil {
				t.Fatal(got.Error)
			}
			d := licenseDetector(ctx, t, modulePath, got.ResolvedVersion, proxyClient)
			fr := cleanFetchResult(test.mod.fr, d)
			sortFetchResult(fr)
			sortFetchResult(got)
			opts := []cmp.Option{
				cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"),
				cmpopts.IgnoreFields(internal.Documentation{}, "HTML"),
				cmpopts.IgnoreFields(internal.PackageVersionState{}, "Error"),
				cmpopts.IgnoreFields(FetchResult{}, "Defer"),
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
			defer got.Defer()
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
