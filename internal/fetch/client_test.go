// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	m := "module"
	v := "v1.5.2"

	if err := c.FetchVersion(ctx, m, v); err != nil {
		t.Errorf("FetchVersion(ctx, %q, %q) = %v; want %v", m, v, err, nil)
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
	m := "module"
	v := "v1.5.2"

	wantErrString := "Bad Request"
	if err := c.FetchVersion(ctx, m, v); !strings.Contains(err.Error(), wantErrString) {
		t.Errorf("FetchVersion(ctx, %q, %q) returned error %v, want error containing %q",
			m, v, err, wantErrString)
	}
}
