// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	expectedStatus := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(expectedStatus)
	}))
	defer server.Close()

	c := New(server.URL)
	request := &Request{
		ModulePath: "module",
		Version:    "v1.5.2",
	}

	if resp := c.FetchVersion(ctx, request); resp.StatusCode != http.StatusOK {
		t.Errorf("FetchVersion(ctx, %v) = %v; want OK", request, resp)
	}
}

func TestFetchVersionInvalidFetchURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	expectedStatus := http.StatusBadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(expectedStatus)
	}))
	defer server.Close()

	c := New(server.URL)
	request := &Request{
		ModulePath: "module",
		Version:    "v1.5.2",
	}

	if resp := c.FetchVersion(ctx, request); resp.StatusCode != expectedStatus {
		t.Errorf("FetchVersion(ctx, %q) returned resp %v, want status %d",
			request, resp, expectedStatus)
	}
}
