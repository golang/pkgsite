// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAcceptRequests_Methods(t *testing.T) {
	mw := AcceptRequests("GET", "HEAD")
	var called bool
	ts := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})))
	defer ts.Close()
	c := ts.Client()

	for _, test := range []struct {
		method string
		want   bool
	}{
		{"GET", true},
		{"HEAD", true},
		{"POST", false},
		{"DELETE", false},
	} {
		called = false
		req, err := http.NewRequest(test.method, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Errorf("%s called: got %t, want %t", test.method, called, test.want)
			continue
		}
		var wantCode int
		if called {
			wantCode = http.StatusOK
		} else {
			wantCode = http.StatusMethodNotAllowed
		}
		if got := res.StatusCode; got != wantCode {
			t.Errorf("%s code: got %d, want %d", test.method, got, wantCode)
		}
	}
}

func TestAcceptRequests_URILength(t *testing.T) {
	mw := AcceptRequests(http.MethodGet)
	var called bool
	ts := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})))
	defer ts.Close()
	c := ts.Client()

	var longURL string
	// Create a URL with 990 characters.
	numParts := maxURILength/2 - 5
	for i := 0; i < numParts; i++ {
		longURL += "/a"
	}
	// Without this query param, the length of longURL will be < maxURILength.
	longURL += "?q=randomstring"
	for _, test := range []struct {
		name, urlPath string
		want          bool
	}{
		{"short URL", "/shorturlpath", true},
		{"long URL", longURL, false},
	} {
		called = false
		req, err := http.NewRequest(http.MethodGet, ts.URL+test.urlPath, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Errorf("%s called: got %t, want %t", test.name, called, test.want)
			continue
		}
		var wantCode int
		if called {
			wantCode = http.StatusOK
		} else {
			wantCode = http.StatusRequestURITooLong
		}
		if got := res.StatusCode; got != wantCode {
			t.Errorf("%s code: got %d, want %d", test.name, got, wantCode)
		}
	}
}
