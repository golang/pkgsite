// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package teeproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestPkgGoDevPath(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{
			path: "/-/about",
			want: "/about",
		},
		{
			path: "/net/http",
			want: "/net/http",
		},
		{
			path: "/",
			want: "/",
		},
		{
			path: "",
			want: "",
		},
	} {
		if got := pkgGoDevPath(test.path); got != test.want {
			t.Fatalf("pkgGoDevPath(%q) = %q; want = %q", test.path, got, test.want)
		}
	}
}

func TestPkgGoDevRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer ts.Close()

	ctx := context.Background()

	got, err := makePkgGoDevRequest(ctx, ts.URL, "")
	if err != nil {
		t.Fatal(err)
	}

	want := &RequestEvent{
		Host:   ts.URL,
		URL:    ts.URL,
		Status: http.StatusOK,
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(RequestEvent{}, "Latency")); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestGetGddoEvent(t *testing.T) {
	for _, test := range []struct {
		gddoEvent *RequestEvent
	}{
		{

			&RequestEvent{
				RedirectHost: "localhost:8080",
				Host:         "godoc.org",
				URL:          "https://godoc.org/net/http",
				Latency:      100,
				Status:       200,
			},
		},
	} {
		requestBody, err := json.Marshal(test.gddoEvent)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/", bytes.NewBuffer(requestBody))
		gotEvent, err := getGddoEvent(r)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(test.gddoEvent, gotEvent); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}
