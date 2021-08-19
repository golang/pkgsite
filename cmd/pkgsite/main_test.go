// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) {
	flag.Set("static", "../../static")
	server, err := newServer(context.Background(), []string{"../../internal/fetch/testdata/has_go_mod"}, false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Install(mux.Handle, nil, nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, httptest.NewRequest("GET", "/example.com/testmod", nil))
	if w.Code != http.StatusOK {
		t.Errorf("%q: got status code = %d, want %d", "/testmod", w.Code, http.StatusOK)
	}
}
