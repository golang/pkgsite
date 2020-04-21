// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

// SetupTestIndex creates a module index for testing using the given version
// map for data.  It returns a function for tearing down the index server after
// the test is completed, and a Client for interacting with the test index.
func SetupTestIndex(t *testing.T, versions []*internal.IndexVersion) (*Client, func()) {
	t.Helper()

	httpClient, server, serverCloseFn := testhelper.SetupTestClientAndServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := len(versions)
			if limitParam := r.FormValue("limit"); limitParam != "" {
				var err error
				limit, err = strconv.Atoi(limitParam)
				if err != nil {
					t.Fatalf("error parsing limit parameter: %v", err)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			for i := 0; i < limit && i < len(versions); i++ {
				json.NewEncoder(w).Encode(versions[i])
			}
		}))

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = httpClient

	fn := func() {
		serverCloseFn()
	}
	return client, fn
}
