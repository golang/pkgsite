// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"cloud.google.com/go/errorreporting"
)

func TestErrorReporting(t *testing.T) {
	tests := []struct {
		code        int
		wantReports int
	}{
		{500, 1},
		{200, 0},
		{404, 0},
		{503, 0},
		{550, 0},
	}

	for _, test := range tests {
		t.Run(strconv.Itoa(test.code), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.code)
			})
			reports := 0
			mw := ErrorReporting(func(errorreporting.Entry) {
				reports++
			})
			ts := httptest.NewServer(mw(handler))
			resp, err := http.Get(ts.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if got := reports; got != test.wantReports {
				t.Errorf("Got %d reports, want %d", got, test.wantReports)
			}
		})
	}
}
