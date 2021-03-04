// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/proxy"
)

func TestLatestModuleVersions(t *testing.T) {
	// latestVersion is tested above.
	// Contents of the go.mod file are tested in proxydatasource.
	// Here, test retractions and presence of a go.mod file.
	prox, teardown := proxy.SetupTestClient(t, testModules)
	defer teardown()

	// These tests depend on the test modules, which are taken from the contents
	// of internal/proxy/testdata/*.txtar.
	for _, test := range []struct {
		modulePath          string
		wantRaw, wantCooked string
	}{
		{"example.com/basic", "v1.1.0", "v1.1.0"},
		{"example.com/retractions", "v1.2.0", "v1.0.0"},
	} {
		got, err := LatestModuleVersions(context.Background(), test.modulePath, prox, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.GoModFile == nil {
			t.Errorf("%s: no go.mod file", test.modulePath)
		}
		if got.RawVersion != test.wantRaw {
			t.Errorf("%s, raw: got %q, want %q", test.modulePath, got.RawVersion, test.wantRaw)
		}
		if got.CookedVersion != test.wantCooked {
			t.Errorf("%s, cooked: got %q, want %q", test.modulePath, got.CookedVersion, test.wantCooked)
		}
	}
}

func TestLatestModuleVersionsNotFound(t *testing.T) {
	// Verify that we get (nil, nil) if there is no version information.
	const modulePath = "example.com/no-versions"
	server := proxy.NewServer(testModules)
	server.AddModuleNoVersions(&proxy.Module{
		ModulePath: modulePath,
		Version:    "v0.0.0-20181107005212-dafb9c8d8707",
	})
	client, teardown, err := proxy.NewClientForServer(server)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	got, err := LatestModuleVersions(context.Background(), modulePath, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
