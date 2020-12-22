// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

var testTimeout = 30 * time.Second

var templateSource = template.TrustedSourceFromConstant("../../content/static/html/doc")

func TestFetchModule(t *testing.T) {
	dochtml.LoadTemplates(templateSource)
	stdlib.UseTestData = true

	// Stub out the function used to share playground snippets
	origPost := httpPost
	httpPost = func(url string, contentType string, body io.Reader) (resp *http.Response, err error) {
		w := httptest.NewRecorder()
		w.WriteHeader(http.StatusOK)
		return w.Result(), nil
	}
	defer func() { httpPost = origPost }()

	defer func(oldmax int) { godoc.MaxDocumentationHTML = oldmax }(godoc.MaxDocumentationHTML)
	godoc.MaxDocumentationHTML = 1 * megabyte

	for _, test := range []struct {
		name         string
		mod          *testModule
		fetchVersion string
		proxyOnly    bool
		cleaned      bool
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
		// Proxy only as stdlib is not accounted for in local mode
		{name: "stdlib module", mod: moduleStd, proxyOnly: true},
		// Proxy only as version is pre specified in local mode
		{name: "master version of module", mod: moduleMaster, fetchVersion: "master", proxyOnly: true},
		// Proxy only as version is pre specified in local mode
		{name: "latest version of module", mod: moduleLatest, fetchVersion: "latest", proxyOnly: true},
	} {
		for _, fetcher := range []struct {
			name  string
			fetch func(t *testing.T, withLicenseDetector bool, ctx context.Context, mod *testModule, fetchVersion string) (*FetchResult, *licenses.Detector)
		}{
			{name: "proxy", fetch: proxyFetcher},
			{name: "local", fetch: localFetcher},
		} {
			if test.proxyOnly && fetcher.name == "local" {
				continue
			}
			t.Run(fmt.Sprintf("%s:%s", fetcher.name, test.name), func(t *testing.T) {
				ctx := context.Background()
				ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()

				got, d := fetcher.fetch(t, true, ctx, test.mod, test.fetchVersion)
				defer got.Defer()
				if got.Error != nil {
					t.Fatal("fetching failed: %w", got.Error)
				}
				if !test.cleaned {
					test.mod.fr = cleanFetchResult(t, test.mod.fr, d)
					test.cleaned = true
				}
				fr := updateFetchResultVersions(t, test.mod.fr, fetcher.name == "local")
				sortFetchResult(fr)
				sortFetchResult(got)
				opts := []cmp.Option{
					cmpopts.IgnoreFields(internal.Documentation{}, "Source"),
					cmpopts.IgnoreFields(internal.PackageVersionState{}, "Error"),
					cmpopts.IgnoreFields(FetchResult{}, "Defer"),
					cmp.AllowUnexported(source.Info{}),
					cmpopts.EquateEmpty(),
				}
				if fetcher.name == "local" {
					opts = append(opts,
						[]cmp.Option{
							// Pre specified for all modules
							cmpopts.IgnoreFields(internal.Module{}, "SourceInfo"),
							cmpopts.IgnoreFields(internal.Module{}, "Version"),
							cmpopts.IgnoreFields(FetchResult{}, "RequestedVersion"),
							cmpopts.IgnoreFields(FetchResult{}, "ResolvedVersion"),
							cmpopts.IgnoreFields(internal.Module{}, "CommitTime"),
						}...)
				}

				opts = append(opts, sample.LicenseCmpOpts...)
				if diff := cmp.Diff(fr, got, opts...); diff != "" {
					t.Fatalf("mismatch (-want +got):\n%s", diff)
				}
				validateDocumentationHTML(t, got.Module, test.mod.docStrings)
			})
		}
	}
}

// validateDocumentationHTML checks that the doc HTMLs for units in the module
// contain a set of substrings.
func validateDocumentationHTML(t *testing.T, got *internal.Module, want map[string][]string) {
	ctx := context.Background()
	for _, u := range got.Units {
		if wantStrings := want[u.Path]; wantStrings != nil {
			parts, err := godoc.RenderPartsFromUnit(ctx, u)
			if err != nil && !errors.Is(err, godoc.ErrTooLarge) {
				t.Fatal(err)
			}
			gotDoc := parts.Body.String()
			for _, w := range wantStrings {
				if !strings.Contains(gotDoc, w) {
					t.Errorf("doc for %s:\nmissing %q; got\n%q", u.Path, w, gotDoc)
				}
			}
		}
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
		for _, fetcher := range []struct {
			name  string
			fetch func(t *testing.T, withLicenseDetector bool, ctx context.Context, mod *testModule, fetchVersion string) (*FetchResult, *licenses.Detector)
		}{
			{name: "proxy", fetch: proxyFetcher},
			{name: "local", fetch: localFetcher},
		} {
			t.Run(fmt.Sprintf("%s:%s", fetcher.name, test.name), func(t *testing.T) {
				got, _ := fetcher.fetch(t, false, ctx, test.mod, "")
				defer got.Defer()
				if !errors.Is(got.Error, test.wantErr) {
					t.Fatalf("got error = %v; wantErr = %v)", got.Error, test.wantErr)
				}
				if test.wantGoModPath != "" {
					if got == nil || got.GoModPath != test.wantGoModPath {
						t.Errorf("got %+v, wanted GoModPath %q", got, test.wantGoModPath)
					}
				}
			})
		}
	}
}
