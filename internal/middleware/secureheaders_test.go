// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecureHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	enableCSP := true
	mw := SecureHeaders(enableCSP)
	ts := httptest.NewServer(mw(handler))
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Errorf("GET returned error %v", err)
	}
	defer resp.Body.Close()
	// Test that the expected headers are set.
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
