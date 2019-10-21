// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQuota(t *testing.T) {
	const burst = 2
	mw := Quota(1, burst, 1)
	var npass int
	h := func(w http.ResponseWriter, r *http.Request) {
		npass++
	}
	ts := httptest.NewServer(mw(http.HandlerFunc(h)))
	defer ts.Close()
	c := ts.Client()

	check := func(msg string, nwant int) {
		npass = 0
		for i := 0; i < 5; i++ {
			req, err := http.NewRequest("GET", ts.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Add("X-Forwarded-For", "1.2.3.4, and more")
			res, err := c.Do(req)
			if err != nil {
				t.Fatalf("%s: %v", msg, err)
			}
			res.Body.Close()
			want := http.StatusOK
			if i >= nwant {
				want = http.StatusTooManyRequests
			}
			if got := res.StatusCode; got != want {
				t.Errorf("%s, #%d: got %d, want %d", msg, i, got, want)
			}
		}
		if npass != nwant {
			t.Errorf("%s: got %d requests to pass, want %d", msg, npass, nwant)
		}
	}

	// When making multiple requests in quick succession from the same IP,
	// only the first two get through; the rest 503.
	check("before", 2)
	// After a second (and a bit more), we should have one token back, meaning
	// we can serve one request.
	time.Sleep(1100 * time.Millisecond)
	check("after", 1)
}

func TestIPKey(t *testing.T) {
	for _, test := range []struct {
		in   string
		want interface{}
	}{
		{"", ""},
		{"1.2.3", ""},
		{"128.197.17.3", "128.197.17.0"},
		{"  128.197.17.3, foo  ", "128.197.17.0"},
		{"2001:db8::ff00:42:8329", "2001:db8::ff00:42:8300"},
	} {
		got := ipKey(test.in)
		if got != test.want {
			t.Errorf("%q: got %v, want %v", test.in, got, test.want)
		}
	}
}
