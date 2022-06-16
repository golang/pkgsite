// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type contextKey int

const key = contextKey(1)

func TestChain(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.Context().Value(key).(int)
		fmt.Fprintf(w, "%d", v)
	})

	add := Middleware(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, _ := r.Context().Value(key).(int)
			ctx := context.WithValue(r.Context(), key, v+2)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	multiply := Middleware(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, _ := r.Context().Value(key).(int)
			ctx := context.WithValue(r.Context(), key, v*2)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	ts := httptest.NewServer(Chain(add, multiply)(handler))
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Fatalf("GET got error %v, want nil", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("io.ReadAll(resp.Body): %v", err)
	}

	// Test that both middleware executed, in the correct order.
	if got, want := string(body), "4"; got != want {
		t.Errorf("GET returned body %q, want %q", got, want)
	}
}
