// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// SetupTestProxyClient creates a module proxy for testing using static files
// stored in internal/proxy/testdata/modproxy/proxy. It returns a function
// for tearing down the proxy after the test is completed and a Client for
// interacting with the test proxy. The following module versions are supported
// by the proxy: (1) my/module v1.0.0 (2) my/module v1.1.0 (3) my/module v1.1.1
// (4) my/module/v2 v2.0.0.
func SetupTestProxyClient(t *testing.T) (func(t *testing.T), *Client) {
	t.Helper()
	proxyDataDir := "../proxy/testdata/modproxy/proxy"
	absPath, err := filepath.Abs(proxyDataDir)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", proxyDataDir, err)
	}

	p := httptest.NewServer(http.FileServer(http.Dir(absPath)))
	client := New(p.URL)

	expectedVersions := [][]string{
		[]string{"my/module", "v1.0.0"},
		[]string{"my/module", "v1.1.0"},
		[]string{"my/module", "v1.1.1"},
		[]string{"my/module/v2", "v2.0.0"},
	}

	for _, v := range expectedVersions {
		if _, err := client.GetInfo(v[0], v[1]); err != nil {
			t.Fatalf("client.GetInfo(%q, %q): %v", v[0], v[1], err)
		}
	}

	fn := func(t *testing.T) {
		p.Close()
	}
	return fn, client
}
