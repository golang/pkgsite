// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentSecurityPolicy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})
	mw := ContentSecurityPolicy()
	ts := httptest.NewServer(mw(handler))
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Errorf("GET returned error %v", err)
	}
	// Simply test that the content security policy is actually set.
	if got := resp.Header.Get("content-security-policy"); got == "" {
		t.Errorf("GET returned empty content-security-policy")
	}
}
