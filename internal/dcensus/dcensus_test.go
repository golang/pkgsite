// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dcensus

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
)

func TestRouter(t *testing.T) {
	view.Register(ServerResponseCount)
	handler := func(w http.ResponseWriter, r *http.Request) {}
	tagger := func(route string, r *http.Request) string {
		tag := strings.Trim(route, "/")
		if addon := r.FormValue("tag"); addon != "" {
			tag += "-" + addon
		}
		return tag
	}
	router := NewRouter(tagger)
	router.HandleFunc("/A/", handler)
	router.HandleFunc("/B/", handler)
	ts := httptest.NewServer(router)
	defer ts.Close()

	requests := []string{"/A/B/C", "/B/A/C", "/A/", "/A/B?tag=special"}
	for _, request := range requests {
		url := ts.URL + request
		resp, err := ts.Client().Get(url)
		if err != nil {
			t.Errorf("GET %s got error %v, want nil", url, err)
		}
		resp.Body.Close()
	}
	rows, err := view.RetrieveData(ServerResponseCount.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"A": 2, "B": 1, "A-special": 1}
	got := make(map[string]int64)
	for _, row := range rows {
		found := false
		for _, tag := range row.Tags {
			if tag.Key == ochttp.KeyServerRoute {
				found = true
				got[tag.Value] = row.Data.(*view.CountData).Value
				break
			}
		}
		if !found {
			t.Fatalf("missing route tag from %v", row)
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected route tag counts (-want +got):\n%s", diff)
	}
}
