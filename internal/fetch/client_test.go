// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchVersion(t *testing.T) {
	expectedStatus := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(expectedStatus)
	}))
	defer server.Close()

	c := New(server.URL)
	m := "module"
	v := "v1.5.2"

	if err := c.FetchVersion(m, v); err != nil {
		t.Errorf("fetchVersion(%q, %q, %q) = %v; want %v", server.URL, m, v, err, nil)
	}
}

func TestFetchVersionInvalidFetchURL(t *testing.T) {
	expectedStatus := http.StatusBadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(expectedStatus)
	}))
	defer server.Close()

	c := New(server.URL)
	m := "module"
	v := "v1.5.2"

	expectedData := map[string]string{"name": m, "version": v}
	expectedErr := fmt.Errorf("http.Post(%q, %q, %q) returned response: 400 (%q)",
		server.URL, "application/json", expectedData, "400 Bad Request")

	if err := c.FetchVersion(m, v); err.Error() != expectedErr.Error() {
		t.Errorf("fetchVersion(%q, %q, %q) = %v; want %v", server.URL, m, v, err, expectedErr)
	}
}
