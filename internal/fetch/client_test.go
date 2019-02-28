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

	url := fmt.Sprintf("%s/%s/@v/%s", server.URL, m, v)
	wantErr := fmt.Errorf(`http.Get(%q) returned response: 400 ("400 Bad Request")`, url)
	if err := c.FetchVersion(m, v); err.Error() != wantErr.Error() {
		t.Errorf("fetchVersion(%q) = %v; want %v", url, err, wantErr)
	}
}
