// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	const origBody = `
    <link foo>
    <script nonce="$$GODISCOVERYNONCE$$" async src="bar"></script>
    blah blah blah
    <script nonce="$$GODISCOVERYNONCE$$">js</script>
    bloo bloo bloo
    <iframe nonce="$$GODISCOVERYNONCE$$" src="baz"></iframe>
`

	const wantBodyFmt = `
    <link foo>
    <script nonce="%[1]s" async src="bar"></script>
    blah blah blah
    <script nonce="%[1]s">js</script>
    bloo bloo bloo
    <iframe nonce="%[1]s" src="baz"></iframe>
`

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, origBody)
	})
	mw := SecureHeaders()
	ts := httptest.NewServer(mw(handler))
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Errorf("GET returned error %v", err)
	}
	defer resp.Body.Close()
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

	// Check that the nonce was substituted correctly.
	// We need to extract it from the header.
	nonceRE := regexp.MustCompile(`'nonce-([^']+)'`)
	matches := nonceRE.FindStringSubmatch(resp.Header.Get("content-security-policy"))
	if matches == nil {
		t.Fatal("cannot extract nonce")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	gotBody := string(body)
	wantBody := fmt.Sprintf(wantBodyFmt, matches[1])
	if gotBody != wantBody {
		t.Errorf("got  body %s\nwant body %s", gotBody, wantBody)
	}
}
