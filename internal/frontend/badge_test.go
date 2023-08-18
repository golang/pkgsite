// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBadgeHandler_ServeSVG(t *testing.T) {
	_, handler := newTestServer(t, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/badge/net/http", nil))
	if got, want := w.Result().Header.Get("Content-Type"), "image/svg+xml"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func TestBadgeHandler_ServeBadgeTool(t *testing.T) {
	_, handler := newTestServer(t, nil)

	tests := []struct {
		url  string
		want string
	}{
		{
			"/badge/",
			`<p class="go-textSubtle">Type a pkg.go.dev URL above to create a badge link.</p>`,
		},
		{
			"/badge/?path=net/http",
			`<a href="https://pkg.go.dev/net/http"><img src="https://pkg.go.dev/badge/net/http.svg" alt="Go Reference"></a>`,
		},
		{
			"/badge/?path=https://pkg.go.dev/net/http",
			`<a href="https://pkg.go.dev/net/http"><img src="https://pkg.go.dev/badge/net/http.svg" alt="Go Reference"></a>`,
		},
		{
			"/badge/?path=github.com/google/uuid",
			"[![Go Reference](https://pkg.go.dev/badge/github.com/google/uuid.svg)](https://pkg.go.dev/github.com/google/uuid)",
		},
		{
			"/badge/?path=https://pkg.go.dev/github.com/google/uuid",
			"[![Go Reference](https://pkg.go.dev/badge/github.com/google/uuid.svg)](https://pkg.go.dev/github.com/google/uuid)",
		},
		{
			"/badge/?path=https://github.com/google/uuid",
			"[![Go Reference](https://pkg.go.dev/badge/github.com/google/uuid.svg)](https://pkg.go.dev/github.com/google/uuid)",
		},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", test.url, nil))
			got := w.Body.String()
			if !strings.Contains(w.Body.String(), test.want) {
				t.Errorf("Expected html substring not found, want %s, got %s", test.want, got)
			}
		})
	}
}
