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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

var testTimeout = 30 * time.Second

var (
	templateSource = template.TrustedSourceFromConstant("../../static/doc")
	testModules    []*proxy.Module
)

type fetchFunc func(t *testing.T, withLicenseDetector bool, ctx context.Context, mod *proxy.Module, fetchVersion string) (*FetchResult, *licenses.Detector)

func TestMain(m *testing.M) {
	dochtml.LoadTemplates(templateSource)
	testModules = proxy.LoadTestModules("../proxy/testdata")
	licenses.OmitExceptions = true
	os.Exit(m.Run())
}

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

	defer func(oldmax int) { godoc.MaxDocumentationHTML = oldmax }(godoc.MaxDocumentationHTML)
	godoc.MaxDocumentationHTML = megabyte / 2

	for _, test := range []struct {
		name         string
		mod          *testModule
		fetchVersion string
		proxyOnly    bool
	}{
		{name: "single", mod: moduleOnePackage},
		{name: "wasm", mod: moduleWasm},
		{name: "no go.mod file", mod: moduleNoGoMod},
		{name: "multi", mod: moduleMultiPackage},
		{name: "bad packages", mod: moduleBadPackages},
		{name: "build constraints", mod: moduleBuildConstraints},
		{name: "bad build context", mod: moduleBadBuildContext},
		{name: "packages with bad import paths", mod: moduleBadImportPath},
		{name: "documentation", mod: moduleDocTest},
		{name: "documentation too large", mod: moduleDocTooLarge},
		{name: "package-level example", mod: modulePackageExample},
		{name: "function example", mod: moduleFuncExample},
		{name: "type example", mod: moduleTypeExample},
		{name: "method example", mod: moduleMethodExample},
		{name: "nonredistributable packages", mod: moduleNonRedist},
		// Proxy only as stdlib is not accounted for in local mode
		{name: "stdlib module", mod: moduleStd, proxyOnly: true},
		// Proxy only as version is pre specified in local mode
		{name: "master version of stdlib module", mod: moduleStdMaster, fetchVersion: "master", proxyOnly: true},
		// Proxy only as version is pre specified in local mode
		{name: "master version of module", mod: moduleMaster, fetchVersion: "master", proxyOnly: true},
		// Proxy only as version is pre specified in local mode
		{name: "latest version of module", mod: moduleLatest, fetchVersion: "latest", proxyOnly: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			mod := test.mod.mod
			if mod == nil {
				mod = test.mod.modfunc()
			}
			if mod == nil {
				t.Fatal("nil module")
			}
			test.mod.fr = cleanFetchResult(t, test.mod.fr)

			for _, fetcher := range []struct {
				name  string
				fetch fetchFunc
			}{
				{name: "proxy", fetch: proxyFetcher},
				{name: "local", fetch: localFetcher},
			} {
				if test.proxyOnly && fetcher.name == "local" {
					continue
				}
				t.Run(fetcher.name, func(t *testing.T) {
					ctx := context.Background()
					ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
					defer cancel()

					got, d := fetcher.fetch(t, true, ctx, mod, test.fetchVersion)
					defer got.Defer()
					if got.Error != nil {
						t.Fatalf("fetching failed: %v", got.Error)
					}
					test.mod.fr = cleanLicenses(t, test.mod.fr, d)
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
		})
	}
}

// validateDocumentationHTML checks that the doc HTMLs for units in the module
// contain a set of substrings.
func validateDocumentationHTML(t *testing.T, got *internal.Module, want map[string][]string) {
	ctx := context.Background()
	for _, u := range got.Units {
		if wantStrings := want[u.Path]; wantStrings != nil {
			parts, err := godoc.RenderFromUnit(ctx, u)
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
		wantHasGoMod  bool
	}{
		{
			name:          "alternative",
			mod:           moduleAlternative,
			wantErr:       derrors.AlternativeModule,
			wantGoModPath: "canonical",
			wantHasGoMod:  true,
		},
		{
			name:          "empty module",
			mod:           moduleEmpty,
			wantErr:       derrors.BadModule,
			wantGoModPath: "emp.ty/module",
			wantHasGoMod:  false,
		},
		{
			name:          "go.mod but no go files",
			mod:           moduleNoGo,
			wantErr:       derrors.BadModule,
			wantGoModPath: "no.go/files",
			wantHasGoMod:  true,
		},
	} {
		for _, fetcher := range []struct {
			name  string
			fetch fetchFunc
		}{
			{name: "proxy", fetch: proxyFetcher},
			{name: "local", fetch: localFetcher},
		} {
			t.Run(fmt.Sprintf("%s:%s", fetcher.name, test.name), func(t *testing.T) {
				got, _ := fetcher.fetch(t, false, ctx, test.mod.mod, "")
				defer got.Defer()
				if !errors.Is(got.Error, test.wantErr) {
					t.Fatalf("got error = %v; wantErr = %v)", got.Error, test.wantErr)
				}
				if got == nil {
					t.Fatal("got nil")
				}
				if g, w := got.GoModPath, test.wantGoModPath; g != w {
					t.Errorf("GoModPath: got %q, want %q", g, w)
				}
				if g, w := got.HasGoMod, test.wantHasGoMod; g != w {
					t.Errorf("HasGoMod: got %t, want %t", g, w)
				}
			})
		}
	}
}

func TestExtractDeprecatedComment(t *testing.T) {
	for _, test := range []struct {
		name        string
		in          string
		wantHas     bool
		wantComment string
	}{
		{"no comment", `module m`, false, ""},
		{"valid comment",
			`
			// Deprecated: use v2
			module m
		`, true, "use v2"},
		{"take first",
			`
			// Deprecated: use v2
			// Deprecated: use v3
			module m
		`, true, "use v2"},
		{"ignore others",
			`
			// c1
			// Deprecated: use v2
			// c2
			module m
		`, true, "use v2"},
		{"must be capitalized",
			`
			// c1
			// deprecated: use v2
			// c2
			module m
		`, false, ""},
		{"suffix",
			`
			// c1
			module m // Deprecated: use v2
		`, true, "use v2",
		},
	} {
		mf, err := modfile.Parse("test", []byte(test.in), nil)
		if err != nil {
			t.Fatal(err)
		}
		gotHas, gotComment := extractDeprecatedComment(mf)
		if gotHas != test.wantHas || gotComment != test.wantComment {
			t.Errorf("%s: got (%t, %q), want(%t, %q)", test.name, gotHas, gotComment, test.wantHas, test.wantComment)
		}
	}
}
