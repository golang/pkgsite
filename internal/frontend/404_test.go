// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
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
			RequestedVersion: internal.LatestVersion,
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
			fr, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.UnknownModulePath, internal.LatestVersion)
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
			_, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.UnknownModulePath, internal.LatestVersion)
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
			RequestedVersion: internal.LatestVersion,
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
			fr, err := previousFetchStatusAndResponse(ctx, testDB, test.path, test.mod, internal.LatestVersion)
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
			if _, err := previousFetchStatusAndResponse(ctx, testDB, test.path, test.mod, internal.LatestVersion); !errors.Is(err, derrors.NotFound) {
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
			RequestedVersion: internal.LatestVersion,
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
		{"module path not at v1", sample.ModulePath, internal.LatestVersion, sample.ModulePath + "/v4", http.StatusFound},
		{"import path not at v1", sample.ModulePath + "/foo", internal.LatestVersion, sample.ModulePath + "/v4/foo", http.StatusFound},
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
