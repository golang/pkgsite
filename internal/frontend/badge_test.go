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
	_, handler, _ := newTestServer(t, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/badge/net/http", nil))
	if got, want := w.Result().Header.Get("Content-Type"), "image/svg+xml"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func TestBadgeHandler_ServeBadgeTool(t *testing.T) {
	_, handler, _ := newTestServer(t, nil)

	tests := []struct {
		url  string
		want string
	}{
		{
			"/badge/",
			"<p>Type a pkg.go.dev URL above to create a badge link.</p>",
		},
		{
			"/badge/?path=net/http",
			`<a href="https://example.com/net/http"><img src="https://example.com/badge/net/http" alt="PkgGoDev"></a>`,
		},
		{
			"/badge/?path=net/http?tab=imports",
			`<a href="https://example.com/net/http?tab=imports"><img src="https://example.com/badge/net/http?tab=imports" alt="PkgGoDev"></a>`,
		},
		{
			"/badge/?path=https://pkg.go.dev/net/http",
			`<a href="https://example.com/net/http"><img src="https://example.com/badge/net/http" alt="PkgGoDev"></a>`,
		},
		{
			"/badge/?path=https://pkg.go.dev/net/http?tab=imports",
			`<a href="https://example.com/net/http?tab=imports"><img src="https://example.com/badge/net/http?tab=imports" alt="PkgGoDev"></a>`,
		},
		{
			"/badge/?path=github.com/google/uuid",
			"[![PkgGoDev](https://example.com/github.com/google/uuid)](https://example.com/badge/github.com/google/uuid)",
		},
		{
			"/badge/?path=github.com/google/uuid?tab=imports",
			"[![PkgGoDev](https://example.com/github.com/google/uuid?tab=imports)](https://example.com/badge/github.com/google/uuid?tab=imports)",
		},
		{
			"/badge/?path=https://pkg.go.dev/github.com/google/uuid",
			"[![PkgGoDev](https://example.com/github.com/google/uuid)](https://example.com/badge/github.com/google/uuid)",
		},
		{
			"/badge/?path=https://pkg.go.dev/github.com/google/uuid?tab=imports",
			"[![PkgGoDev](https://example.com/github.com/google/uuid?tab=imports)](https://example.com/badge/github.com/google/uuid?tab=imports)",
		},
		{
			"/badge/?path=https://google.com",
			"<p>Type a pkg.go.dev URL above to create a badge link.</p>",
		},
		{
			"/badge/?path=https://google.com/github.com/google/uuid",
			"[![PkgGoDev](https://example.com/github.com/google/uuid)](https://example.com/badge/github.com/google/uuid)",
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
