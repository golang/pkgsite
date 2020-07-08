// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const shareID = "arbitraryShareID"

func TestPlaygroundShare(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Expected a POST", http.StatusMethodNotAllowed)
		}
		_, err := io.WriteString(w, shareID)
		if err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	testCases := []struct {
		pgURL   string
		method  string
		desc    string
		body    string
		code    int
		shareID string
	}{
		{
			pgURL:  ts.URL,
			method: http.MethodPost,
			desc:   "Share endpoint: for Hello World func",
			body: `package main
import (
	"fmt"
)

func main() {
	fmt.Println("Hello, playground")
}`,
			code: http.StatusOK,
		},
		{
			pgURL:  ts.URL,
			method: http.MethodGet,
			desc:   "Share endpoint: Failed GET Request, Accept POST only",
			body:   "",
			code:   http.StatusMethodNotAllowed,
		},
		{
			pgURL:  "/*?",
			method: http.MethodPost,
			desc:   "Share endpoint: Malformed URL returns internal server error",
			body:   "",
			code:   http.StatusInternalServerError,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			body := strings.NewReader(tc.body)

			req, err := http.NewRequest(tc.method, "/play", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
			w := httptest.NewRecorder()
			makeFetchPlayRequest(w, req, tc.pgURL)

			res := w.Result()

			if got, want := res.StatusCode, tc.code; got != want {
				t.Errorf("Status Code = %d; want %d", got, want)
			}

			if res.StatusCode >= 200 && res.StatusCode < 300 {
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					t.Fatal(err)
				}

				if string(body) != shareID {
					t.Errorf("body = %s; want %s", body, shareID)
				}
			}
		})
	}
}
