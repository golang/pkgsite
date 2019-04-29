// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// insecureHTTPClient is used to disable TLS verification when running against
// a test server.
var insecureHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

// SetupTestIndex creates a module index for testing using the given version
// map for data.  It returns a function for tearing down the index server after
// the test is completed, and a Client for interacting with the test index.
func SetupTestIndex(t *testing.T, versions []map[string]string) (func(t *testing.T), *Client) {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, v := range versions {
			json.NewEncoder(w).Encode(v)
		}
	}))

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("index.New(%q): %v", server.URL, err)
	}
	client.httpClient = insecureHTTPClient

	fn := func(t *testing.T) {
		server.Close()
	}
	return fn, client
}
