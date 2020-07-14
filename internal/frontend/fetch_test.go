// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var (
	sourceTimeout       = 1 * time.Second
	testModulePath      = "github.com/module"
	testSemver          = "v1.5.2"
	testFetchTimeout    = 100 * time.Second
	testModulesForProxy = []*proxy.TestModule{
		{
			ModulePath: testModulePath,
			Version:    testSemver,
			Files: map[string]string{
				"bar/foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
				"README.md":      "This is a readme",
				"LICENSE":        testhelper.MITLicense,
			},
		},
	}
)

func TestFetch(t *testing.T) {
	for _, test := range []struct {
		name, fullPath, version, want string
	}{
		{
			name:     "path at master package is in module root",
			fullPath: testModulePath,
			version:  internal.MasterVersion,
		},
		{
			name:     "path at latest package is in module root",
			fullPath: testModulePath,
			version:  internal.LatestVersion,
		},
		{
			name:     "path at semver package is in module root",
			fullPath: testModulePath,
			version:  "v1.5.2",
		},
		{
			name:     "package at latest package is not in module root",
			fullPath: testModulePath + "/bar/foo",
			version:  internal.LatestVersion,
		},
		{
			name:     "directory at master package is not in module root",
			fullPath: testModulePath + "/bar",
			version:  internal.MasterVersion,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _, teardown := newTestServer(t, testModulesForProxy)
			defer teardown()

			ctx, cancel := context.WithTimeout(context.Background(), testFetchTimeout)
			defer cancel()
			ctx = experiment.NewContext(ctx,
				internal.ExperimentFrontendFetch,
				internal.ExperimentMasterVersion,
				internal.ExperimentUsePathInfo)

			status, responseText := s.fetchAndPoll(ctx, testModulePath, test.fullPath, test.version)
			if status != http.StatusOK {
				t.Fatalf("fetchAndPoll(%q, %q, %q) = %d, %s; want status = %d",
					testModulePath, test.fullPath, test.version, status, responseText, http.StatusOK)
			}
		})
	}
}

func TestFetchErrors(t *testing.T) {
	for _, test := range []struct {
		name, modulePath, fullPath, version string
		fetchTimeout                        time.Duration
		want                                int
	}{
		{
			name:       "non-existent module",
			modulePath: "github.com/nonexistent",
			fullPath:   "github.com/nonexistent",
			version:    internal.LatestVersion,
			want:       http.StatusNotFound,
		},
		{
			name:       "version invalid",
			modulePath: testModulePath,
			fullPath:   testModulePath,
			version:    "random-version",
			want:       http.StatusBadRequest,
		},
		{
			name:       "module found but package does not exist",
			modulePath: testModulePath,
			fullPath:   "github.com/module/pkg-nonexistent",
			version:    internal.LatestVersion,
			want:       http.StatusNotFound,
		},
		{
			name:         "module exists but fetch timed out",
			modulePath:   testModulePath,
			fullPath:     testModulePath,
			version:      internal.LatestVersion,
			fetchTimeout: 1 * time.Millisecond,
			want:         http.StatusRequestTimeout,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.fetchTimeout == 0 {
				test.fetchTimeout = testFetchTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), test.fetchTimeout)
			defer cancel()

			ctx = experiment.NewContext(ctx, internal.ExperimentFrontendFetch)
			s, _, teardown := newTestServer(t, testModulesForProxy)
			defer teardown()
			got, _ := s.fetchAndPoll(ctx, test.modulePath, test.fullPath, test.version)
			if got != test.want {
				t.Fatalf("fetchAndPoll(ctx, testDB, q, %q, %q, %q): %d; want = %d",
					test.modulePath, test.fullPath, test.version, got, test.want)
			}
		})
	}
}

func TestFetchPathAlreadyExists(t *testing.T) {
	for _, test := range []struct {
		status, want int
	}{
		{http.StatusOK, http.StatusOK},
		{http.StatusNotFound, http.StatusNotFound},
		{derrors.ToHTTPStatus(derrors.AlternativeModule), http.StatusSeeOther},
	} {
		t.Run(strconv.Itoa(test.status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testFetchTimeout)
			defer cancel()
			ctx = experiment.NewContext(ctx,
				internal.ExperimentFrontendFetch,
			)
			if err := testDB.InsertModule(ctx, sample.DefaultModule()); err != nil {
				t.Fatal(err)
			}
			if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
				ModulePath:       sample.ModulePath,
				RequestedVersion: sample.VersionString,
				ResolvedVersion:  sample.VersionString,
				Status:           test.status,
			}); err != nil {
				t.Fatal(err)
			}

			s, _, teardown := newTestServer(t, testModulesForProxy)
			defer teardown()
			got, _ := s.fetchAndPoll(ctx, sample.ModulePath, sample.PackagePath, sample.VersionString)
			if got != test.want {
				t.Fatalf("fetchAndPoll for status %d: %d; want = %d)", test.status, got, test.want)
			}
		})
	}
}
