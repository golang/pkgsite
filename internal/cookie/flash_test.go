// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const (
	testName = "testname"
	testVal  = "testvalue"
)

func TestFlashMessages(t *testing.T) {
	w := httptest.NewRecorder()

	Set(w, testName, testVal, "/foo")
	r := &http.Request{
		Header: http.Header{"Cookie": w.Header()["Set-Cookie"]},
		URL:    &url.URL{Path: "/foo"},
	}

	got, err := Extract(w, r, testName)
	if err != nil {
		t.Fatal(err)
	}
	if got != testVal {
		t.Errorf("got %q, want %q", got, testVal)
	}
}
