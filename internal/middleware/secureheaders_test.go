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

func TestPolicySerialization(t *testing.T) {
	var p policy
	p.add("default-src", "'self'", "example.com")
	p.add("img-src", "*")
	want := "default-src 'self' example.com; img-src *"
	if got := p.serialize(); got != want {
		t.Errorf("p.serialize() = %s, want %s", got, want)
	}
}

func TestSecureHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})
	mw := SecureHeaders()
	ts := httptest.NewServer(mw(handler))
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Errorf("GET returned error %v", err)
	}
	// Simply test that the expected headers are set.
	expectedHeaders := []string{
		"content-security-policy",
		"x-frame-options",
		"x-content-type-options",
	}
	for _, header := range expectedHeaders {
		if got := resp.Header.Get(header); got == "" {
			t.Errorf("GET returned empty %s", header)
		}
	}

}
