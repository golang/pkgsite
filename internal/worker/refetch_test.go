// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func TestReFetch(t *testing.T) {
	// This test checks that re-fetching a version will cause its data to be
	// overwritten.  This is achieved by fetching against two different versions
	// of the (fake) proxy, though in reality the most likely cause of changes to
	// a version is updates to our data model or fetch logic.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	var (
		modulePath = sample.ModulePath
		version    = sample.VersionString
		pkgFoo     = sample.ModulePath + "/foo"
		foo        = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
		pkgBar = sample.ModulePath + "/bar"
		foobar = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
			"bar/bar.go": "// Package bar\npackage bar\n\nconst Bar = 21",
		}
	)

	// First fetch and insert a version containing package foo, and verify that
	// foo can be retrieved.
	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()
	sourceClient := source.NewClient(sourceTimeout)
	if _, err := FetchAndUpdateState(ctx, sample.ModulePath, version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", sample.ModulePath, version, proxyClient, sourceClient, testDB, err)
	}

	if _, err := testDB.LegacyGetPackage(ctx, pkgFoo, internal.UnknownModulePath, version); err != nil {
		t.Error(err)
	}

	// Now re-fetch and verify that contents were overwritten.
	proxyClient, teardownProxy = proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: sample.ModulePath,
			Version:    version,
			Files:      foobar,
		},
	})
	defer teardownProxy()

	if _, err := FetchAndUpdateState(ctx, sample.ModulePath, version, proxyClient, sourceClient, testDB, testAppVersion); err != nil {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
	want := &internal.LegacyVersionedPackage{
		LegacyModuleInfo: internal.LegacyModuleInfo{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        sample.ModulePath,
				Version:           version,
				CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
				IsRedistributable: true,
				HasGoMod:          false,
				SourceInfo:        source.NewGitHubInfo("https://"+sample.ModulePath, "", sample.VersionString),
			},
			LegacyReadmeFilePath: "README.md",
			LegacyReadmeContents: "This is a readme",
		},
		LegacyPackage: internal.LegacyPackage{
			Path:              sample.ModulePath + "/bar",
			Name:              "bar",
			Synopsis:          "Package bar",
			DocumentationHTML: html("Bar returns the string &#34;bar&#34;."),
			V1Path:            sample.ModulePath + "/bar",
			Licenses: []*licenses.Metadata{
				{Types: []string{"MIT"}, FilePath: "LICENSE"},
			},
			IsRedistributable: true,
			GOOS:              "linux",
			GOARCH:            "amd64",
		},
	}
	got, err := testDB.LegacyGetPackage(ctx, pkgBar, internal.UnknownModulePath, version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"), cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Errorf("testDB.LegacyGetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", pkgBar, version, diff)
	}

	// Now re-fetch and verify that contents were overwritten.
	proxyClient, teardownProxy = proxy.SetupTestClient(t, []*proxy.Module{
		{
			ModulePath: modulePath,
			Version:    version,
			Files:      foo,
		},
	})
	defer teardownProxy()
	if _, err := FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, testDB, testAppVersion); !errors.Is(err, derrors.DBModuleInsertInvalid) {
		t.Fatalf("FetchAndUpdateState(%q, %q, %v, %v, %v): %v", modulePath, version, proxyClient, sourceClient, testDB, err)
	}
}
