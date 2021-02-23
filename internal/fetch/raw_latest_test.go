// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal/proxy"
)

func TestFetchRawLatestVersion(t *testing.T) {
	prox, teardown := proxy.SetupTestClient(t, testModules)
	defer teardown()

	for _, test := range []struct {
		module string
		want   string
	}{
		{"example.com/basic", "v1.1.0"},
		{"example.com/single", "v1.0.0"},
	} {
		got, err := fetchRawLatestVersion(context.Background(), test.module, prox, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("%s: got %s, want %s", test.module, got, test.want)
		}
	}
}

func TestRawLatestInfo(t *testing.T) {
	// fetchRawLatestVersion is tested above.
	// Contents of the go.mod file are tested in proxydatasource.
	// Here, just test that there is a parsed go.mod file.
	prox, teardown := proxy.SetupTestClient(t, testModules)
	defer teardown()

	const module = "example.com/basic"
	got, err := RawLatestInfo(context.Background(), module, prox, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModulePath != module || got.Version != "v1.1.0" || got.GoModFile == nil {
		t.Errorf("got (%q, %q, %p), want (%q, 'v1.1.0', <non-nil>)", got.ModulePath, got.Version, got.GoModFile, module)
	}

}
