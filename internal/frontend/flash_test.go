// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlashMessages(t *testing.T) {
	w := httptest.NewRecorder()

	testVal := "value"
	setFlashMessage(w, alternativeModuleFlash, testVal, "/")
	r := &http.Request{Header: http.Header{"Cookie": w.Header()["Set-Cookie"]}}

	got, err := getFlashMessage(w, r, alternativeModuleFlash)
	if err != nil {
		t.Fatal(err)
	}
	if got != testVal {
		t.Errorf("got %q, want %q", got, testVal)
	}
}
