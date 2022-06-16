// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var playground = flag.Bool("playground", false, "Make a request to https://play.golang.org/")

const testShareID = "arbitraryShareID"

func TestPlaygroundShare(t *testing.T) {
	pgURL := playgroundURL
	if !*playground {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Expected a POST", http.StatusMethodNotAllowed)
			}

			if _, err := io.WriteString(w, testShareID); err != nil {
				t.Fatal(err)
			}
		}))
		defer ts.Close()
		testURL, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		pgURL = testURL
	}

	testCases := []struct {
		pgURL  *url.URL
		method string
		desc   string
		body   string
		code   int
		// shareID is a hash returned by play.golang.org when the body is POSTed to
		// play.golang.org/share. We expect play.golang.org to always return the
		// same hash for a given unique body. If the request is made to the mock
		// server, shareID will be set to testShareID when running the tests below.
		shareID string
	}{
		{
			pgURL:  pgURL,
			method: http.MethodPost,
			desc:   "Share endpoint: for Hello World func",
			body: `package main

import (
	"fmt"
)

func main() {
	fmt.Println("Hello, playground")
}`,
			code:    http.StatusOK,
			shareID: "BpXrY1MHLkk",
		},
		{
			pgURL:   pgURL,
			method:  http.MethodGet,
			desc:    "Share endpoint: Failed GET Request, Accept POST only",
			code:    http.StatusMethodNotAllowed,
			shareID: "UCPdVNrl0-P",
		},
		{
			pgURL:  &url.URL{Path: "/*?"},
			method: http.MethodPost,
			desc:   "Share endpoint: Malformed URL returns internal server error",
			code:   http.StatusInternalServerError,
		},
	}
	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			body := strings.NewReader(test.body)

			req, err := http.NewRequest(test.method, "/play/share", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
			w := httptest.NewRecorder()
			proxy := makePlaygroundProxy(test.pgURL)
			proxy.ServeHTTP(w, req)

			res := w.Result()
			if got, want := res.StatusCode, test.code; got != want {
				t.Errorf("Status Code = %d; want %d", got, want)
			}

			if res.StatusCode >= 200 && res.StatusCode < 300 {
				body, err := io.ReadAll(res.Body)
				if err != nil {
					t.Fatal(err)
				}
				wantID := test.shareID
				if !*playground {
					wantID = testShareID
				}
				if string(body) != wantID {
					t.Errorf("body = %s; want %s", body, wantID)
				}
			}
		})
	}
}
