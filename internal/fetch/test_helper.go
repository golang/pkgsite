// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// SetupTestFetch creates a fake fetch service implementing the given
// request->response behavior. It returns a Client pointing to this fake
// server.
func SetupTestFetch(t *testing.T, responses map[Request]*Response) (func(t *testing.T), *Client) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		module, version, err := ParseModulePathAndVersion(r.URL.Path)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		resp := responses[Request{ModulePath: module, Version: version}]
		if resp != nil {
			w.WriteHeader(resp.StatusCode)
			w.Write([]byte(resp.Error))
		} else {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
	}))

	client := New(server.URL)

	fn := func(t *testing.T) {
		server.Close()
	}
	return fn, client
}
