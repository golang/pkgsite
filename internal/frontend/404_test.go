// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/cookie"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

func TestPreviousFetchStatusAndResponse(t *testing.T) {
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)

	for _, mod := range []struct {
		path      string
		goModPath string
		status    int
	}{
		{"mvdan.cc/sh", "", 200},
		{"mvdan.cc", "", 404},
		{"400.mod/foo/bar", "", 400},
		{"400.mod/foo", "", 404},
		{"400.mod", "", 404},
		{"github.com/alternative/ok", "github.com/vanity", 491},
		{"github.com/alternative/ok/path", "", 404},
		{"github.com/alternative/bad", "vanity", 491},
		{"github.com/kubernetes/client-go", "k8s.io/client-go", 491},
		{"bad.mod/foo/bar", "", 490},
		{"bad.mod/foo", "", 404},
		{"bad.mod", "", 490},
		{"500.mod/foo", "", 404},
		{"500.mod", "", 500},
		{"reprocess.mod/foo", "", 520},
	} {
		goModPath := mod.goModPath
		if goModPath == "" {
			goModPath = mod.path
		}
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       mod.path,
			RequestedVersion: version.Latest,
			ResolvedVersion:  sample.VersionString,
			Status:           mod.status,
			GoModPath:        goModPath,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"bad request at path root", "400.mod/foo/bar", 404},
		{"bad request at mod but 404 at path", "400.mod/foo", 404},
		{"alternative mod", "github.com/alternative/ok", 491},
		{"alternative mod package path", "github.com/alternative/ok/path", 491},
		{"alternative mod bad module path", "github.com/alternative/bad", 404},
		{"bad module at path", "bad.mod/foo/bar", 404},
		{"bad module at mod but 404 at path", "bad.mod/foo", 404},
		{"500", "500.mod/foo", 500},
		{"mod to reprocess", "reprocess.mod/foo", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			fr, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.UnknownModulePath, version.Latest)
			if err != nil {
				t.Fatal(err)
			}
			if fr.status != test.status {
				t.Errorf("got %v; want %v", fr.status, test.status)
			}
		})
	}

	for _, test := range []struct {
		name, path string
	}{
		{"path never fetched", "github.com/non/existent"},
		{"path never fetched, but top level mod fetched", "mvdan.cc/sh/v3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.UnknownModulePath, version.Latest)
			if !errors.Is(err, derrors.NotFound) {
				t.Errorf("got %v; want %v", err, derrors.NotFound)
			}
		})
	}
}

func TestPreviousFetchStatusAndResponse_AlternativeModuleWithDeepLinking(t *testing.T) {
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)

	for _, mod := range []struct {
		path      string
		goModPath string
		status    int
	}{
		{"k8s.io/client-go", "k8s.io/client-go", 200},
		{"github.com/kubernetes/client-go", "k8s.io/client-go", 491},
	} {
		goModPath := mod.goModPath
		if goModPath == "" {
			goModPath = mod.path
		}
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       mod.path,
			RequestedVersion: version.Latest,
			ResolvedVersion:  sample.VersionString,
			Status:           mod.status,
			GoModPath:        goModPath,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name, path, mod string
		status          int
	}{
		{"path with specified module", "github.com/kubernetes/client-go/informers/admissionregistration/v1", "github.com/kubernetes/client-go", 491},
	} {
		t.Run(test.name, func(t *testing.T) {
			fr, err := previousFetchStatusAndResponse(ctx, testDB, test.path, test.mod, version.Latest)
			if err != nil {
				t.Fatal(err)
			}
			if fr.status != test.status {
				t.Errorf("got %v; want %v", fr.status, test.status)
			}
		})
	}
	for _, test := range []struct {
		name, path, mod string
	}{
		{"path with unknown module", "github.com/kubernetes/client-go/informers/admissionregistration/v1", internal.UnknownModulePath},
		{"module nonexistent module", "github.com/kubernetes/client-go/typo", "github.com/kubernetes/client-go/typo"},
		{"path with specified nonexistent module", "github.com/kubernetes/client-go/typo/informers/admissionregistration/v1", "github.com/kubernetes/client-go/typo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := previousFetchStatusAndResponse(ctx, testDB, test.path, test.mod, version.Latest); !errors.Is(err, derrors.NotFound) {
				t.Fatal(err)
			}
		})
	}
}

func TestPreviousFetchStatusAndResponse_PathExistsAtNonV1(t *testing.T) {
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)

	postgres.MustInsertModule(ctx, t, testDB, sample.Module(sample.ModulePath+"/v4", "v4.0.0", "foo"))

	for _, mod := range []struct {
		path, version string
		status        int
	}{
		{sample.ModulePath, "v1.0.0", http.StatusNotFound},
		{sample.ModulePath + "/foo", "v4.0.0", http.StatusNotFound},
		{sample.ModulePath + "/v4", "v4.0.0", http.StatusOK},
	} {
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       mod.path,
			RequestedVersion: version.Latest,
			ResolvedVersion:  mod.version,
			Status:           mod.status,
			GoModPath:        mod.path,
		}); err != nil {
			t.Fatal(err)
		}
	}

	checkPath := func(ctx context.Context, t *testing.T, testDB *postgres.DB, path, version, wantPath string, wantStatus int) {
		got, err := previousFetchStatusAndResponse(ctx, testDB, path, internal.UnknownModulePath, version)
		if err != nil {
			t.Fatal(err)
		}
		want := &fetchResult{
			modulePath: wantPath,
			goModPath:  wantPath,
			status:     wantStatus,
		}
		if diff := cmp.Diff(want, got,
			cmp.AllowUnexported(fetchResult{}),
			cmpopts.IgnoreFields(fetchResult{}, "responseText")); diff != "" {
			t.Errorf("mismatch (-want, +got):\n%s", diff)
		}
	}

	for _, test := range []struct {
		name, path, version, wantPath string
		wantStatus                    int
	}{
		{"module path not at v1", sample.ModulePath, version.Latest, sample.ModulePath + "/v4", http.StatusFound},
		{"import path not at v1", sample.ModulePath + "/foo", version.Latest, sample.ModulePath + "/v4/foo", http.StatusFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkPath(ctx, t, testDB, test.path, test.version, test.wantPath, test.wantStatus)
		})
	}
	for _, test := range []struct {
		name, path, version string
	}{
		{"import path v1 missing version", sample.ModulePath + "/foo", "v1.5.2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.UnknownModulePath, test.version)
			if !errors.Is(err, derrors.NotFound) {
				t.Fatal(err)
			}
		})
	}
}

func TestGithubPathRedirect(t *testing.T) {
	for _, test := range []struct {
		path, want string
	}{
		{sample.ModulePath, ""},
		{sample.ModulePath + "/tree/master/tree/master", ""},
		{sample.ModulePath + "/blob", "/" + sample.ModulePath},
		{sample.ModulePath + "/tree", "/" + sample.ModulePath},
		{sample.ModulePath + "/blob/master", "/" + sample.ModulePath},
		{sample.ModulePath + "/tree/master", "/" + sample.ModulePath},
		{sample.ModulePath + "/blob/master/pkg", "/" + sample.ModulePath + "/pkg"},
		{sample.ModulePath + "/tree/master/pkg", "/" + sample.ModulePath + "/pkg"},
		{sample.ModulePath + "/blob/v1.0.0/pkg", "/" + sample.ModulePath + "/pkg"},
		{sample.ModulePath + "/tree/v2.0.0/pkg", "/" + sample.ModulePath + "/pkg"},
		{"bitbucket.org/valid/module_name" + "/tree", ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := githubPathRedirect(test.path); got != test.want {
				t.Fatalf("githubPathRedirect(%q): %q; want = %q", test.path, got, "/"+test.want)
			}
		})
	}
}

func TestStdlibPathForShortcut(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	m := sample.Module(stdlib.ModulePath, "v1.2.3",
		"encoding/json",                  // one match for "json"
		"text/template", "html/template", // two matches for "template"
	)
	ctx := context.Background()
	postgres.MustInsertModule(ctx, t, testDB, m)

	for _, test := range []struct {
		path string
		want string
	}{
		{"foo", ""},
		{"json", "encoding/json"},
		{"template", ""},
	} {
		got, err := stdlibPathForShortcut(ctx, testDB, test.path)
		if err != nil {
			t.Fatalf("%q: %v", test.path, err)
		}
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.path, got, test.want)
		}
	}
}

// Verify that some paths that aren't found will redirect to valid pages.
// Sometimes redirection sets the AlternativeModuleFlash cookie and puts
// up a banner.
func TestServer404Redirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	sampleModule := sample.DefaultModule()
	postgres.MustInsertModule(ctx, t, testDB, sampleModule)
	alternativeModule := &internal.VersionMap{
		ModulePath:       "module.path/alternative",
		GoModPath:        sample.ModulePath,
		RequestedVersion: version.Latest,
		ResolvedVersion:  sample.VersionString,
		Status:           derrors.ToStatus(derrors.AlternativeModule),
	}
	if err := testDB.UpsertVersionMap(ctx, alternativeModule); err != nil {
		t.Fatal(err)
	}

	v1modpath := "notinv1.mod"
	v1path := "notinv1.mod/foo"
	postgres.MustInsertModule(ctx, t, testDB, sample.Module(v1modpath+"/v4", "v4.0.0", "foo"))
	for _, mod := range []struct {
		path, version string
		status        int
	}{
		{v1modpath, "v1.0.0", http.StatusNotFound},
		{v1path, "v4.0.0", http.StatusNotFound},
		{v1modpath + "/v4", "v4.0.0", http.StatusOK},
	} {
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       mod.path,
			RequestedVersion: version.Latest,
			ResolvedVersion:  mod.version,
			Status:           mod.status,
			GoModPath:        mod.path,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
		ModulePath:       sample.ModulePath + "/blob/master",
		RequestedVersion: version.Latest,
		ResolvedVersion:  sample.VersionString,
		Status:           http.StatusNotFound,
	}); err != nil {
		t.Fatal(err)
	}

	rs, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()

	_, _, handler, _ := newTestServerWithFetch(t, nil, middleware.NewCacher(redis.NewClient(&redis.Options{Addr: rs.Addr()})))

	for _, test := range []struct {
		name, path, flash string
	}{
		{"github url", "/" + sample.ModulePath + "/blob/master", ""},
		{"alternative module", "/" + alternativeModule.ModulePath, "module.path/alternative"},
		{"module not in v1", "/" + v1modpath, "notinv1.mod"},
		{"import path not in v1", "/" + v1path, "notinv1.mod/foo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
			// Check for http.StatusFound, which indicates a redirect.
			if w.Code != http.StatusFound {
				t.Errorf("%q: got status code = %d, want %d", test.path, w.Code, http.StatusFound)
			}
			res := w.Result()
			c := findCookie(cookie.AlternativeModuleFlash, res.Cookies())
			if c == nil && test.flash != "" {
				t.Error("got no flash cookie, expected one")
			} else if c != nil {
				val, err := cookie.Base64Value(c)
				if err != nil {
					t.Fatal(err)
				}
				if val != test.flash {
					t.Fatalf("got cookie value %q, want %q", val, test.flash)
				}
				// If we have a cookie, then following the redirect URL with the cookie
				// should serve a "redirected from" banner.
				loc := res.Header.Get("Location")
				r := httptest.NewRequest("GET", loc, nil)
				r.AddCookie(c)
				w = httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				err = checkBody(w.Result().Body, in(`[data-test-id="redirected-banner-text"]`, hasText(val)))
				if err != nil {
					t.Fatal(err)
				}
				// Visiting the same page again without the cookie should not
				// display the banner.
				r = httptest.NewRequest("GET", loc, nil)
				w = httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				err = checkBody(w.Result().Body, notIn(`[data-test-id="redirected-banner-text"]`))
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestServer404Redirect_NoLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	altPath := "module.path/alternative"
	goModPath := "module.path/alternative/pkg"
	defer postgres.ResetTestDB(testDB, t)
	sampleModule := sample.DefaultModule()
	postgres.MustInsertModule(ctx, t, testDB, sampleModule)
	alternativeModule := &internal.VersionMap{
		ModulePath:       altPath,
		GoModPath:        goModPath,
		RequestedVersion: version.Latest,
		ResolvedVersion:  sample.VersionString,
		Status:           derrors.ToStatus(derrors.AlternativeModule),
	}
	alternativeModulePkg := &internal.VersionMap{
		ModulePath:       goModPath,
		GoModPath:        goModPath,
		RequestedVersion: version.Latest,
		ResolvedVersion:  sample.VersionString,
		Status:           http.StatusNotFound,
	}
	if err := testDB.UpsertVersionMap(ctx, alternativeModule); err != nil {
		t.Fatal(err)
	}
	if err := testDB.UpsertVersionMap(ctx, alternativeModulePkg); err != nil {
		t.Fatal(err)
	}

	rs, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()

	_, _, handler, _ := newTestServerWithFetch(t, nil, middleware.NewCacher(redis.NewClient(&redis.Options{Addr: rs.Addr()})))

	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"do not redirect if alternative module does not successfully return", "/" + altPath, http.StatusNotFound},
		{"do not redirect go mod path endlessly", "/" + goModPath, http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
			// Check for http.StatusFound, which indicates a redirect.
			if w.Code != test.status {
				t.Errorf("%q: got status code = %d, want %d", test.path, w.Code, test.status)
			}
		})
	}
}

func TestEmptyDirectoryBetweenNestedModulesRedirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	postgres.MustInsertModule(ctx, t, testDB, sample.Module(sample.ModulePath, sample.VersionString, ""))
	postgres.MustInsertModule(ctx, t, testDB, sample.Module(sample.ModulePath+"/missing/dir/c", sample.VersionString, ""))

	missingPath := sample.ModulePath + "/missing"
	notInsertedPath := sample.ModulePath + "/missing/dir"
	if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
		ModulePath:       missingPath,
		RequestedVersion: version.Latest,
		ResolvedVersion:  sample.VersionString,
	}); err != nil {
		t.Fatal(err)
	}

	_, _, handler, _ := newTestServerWithFetch(t, nil, nil)
	for _, test := range []struct {
		name, path   string
		wantStatus   int
		wantLocation string
	}{
		{"want 404 for unknown version of module", sample.ModulePath + "@v0.5.0", http.StatusNotFound, ""},
		{"want 404 for never fetched directory", notInsertedPath, http.StatusNotFound, ""},
		{"want 302 for previously fetched directory", missingPath, http.StatusFound, "/search?q=" + url.PathEscape(missingPath)},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", "/"+test.path, nil))
			if w.Code != test.wantStatus {
				t.Errorf("%q: got status code = %d, want %d", "/"+test.path, w.Code, test.wantStatus)
			}
			if got := w.Header().Get("Location"); got != test.wantLocation {
				t.Errorf("got location = %q, want %q", got, test.wantLocation)
			}
		})
	}
}

func TestServerErrors(t *testing.T) {
	_, _, handler, _ := newTestServerWithFetch(t, nil, nil)
	for _, test := range []struct {
		name, path string
		wantCode   int
	}{
		{"not found", "/invalid-page", http.StatusNotFound},
		{"bad request", "/gocloud.dev/@latest/blob", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
			if w.Code != test.wantCode {
				t.Errorf("%q: got status code = %d, want %d", test.path, w.Code, test.wantCode)
			}
		})
	}
}
