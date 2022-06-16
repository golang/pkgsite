// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanic(t *testing.T) {
	var doPanic bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if doPanic {
			panic("panic!")
		}
		fmt.Fprint(w, "ok")
	})
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "don't panic")
	})
	mw := Panic(panicHandler)
	ts := httptest.NewServer(mw(handler))

	tests := []struct {
		doPanic  bool
		wantBody string
		wantCode int
	}{
		{true, "don't panic", http.StatusInternalServerError},
		{false, "ok", http.StatusOK},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("doPanic=%t", test.doPanic), func(t *testing.T) {
			doPanic = test.doPanic
			resp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != test.wantCode {
				t.Errorf("code=%d, want %d", resp.StatusCode, test.wantCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(body); got != test.wantBody {
				t.Errorf("body=%q, want %q", got, test.wantBody)
			}
		})
	}
}
