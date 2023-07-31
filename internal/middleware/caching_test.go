// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/alicebob/miniredis/v2 pulls in
// github.com/yuin/gopher-lua which uses a non
// build-tag-guarded use of the syscall package.
//go:build !plan9

package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/stats/view"
	"golang.org/x/pkgsite/internal/config"
)

func TestCache(t *testing.T) {
	// force cache writes to be synchronous
	TestMode = true
	// These variables are mutated before each test case to control the handler
	// response.
	var (
		body   string
		status int
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status > 0 {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	})

	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	mux := http.NewServeMux()
	mux.Handle("/A", NewCacher(c).Cache("A", ttl(1*time.Minute), []string{"yes"})(handler))
	mux.Handle("/B", handler)
	ts := httptest.NewServer(mux)
	view.Register(CacheResultCount)
	// The following tests are stateful: the result of each test depends on the
	// state in redis resulting from all tests before it.
	tests := []struct {
		label         string
		advanceTime   time.Duration
		path          string
		body          string
		status        int
		bypass        bool
		wantHitCounts map[bool]int
		wantBody      string
		wantStatus    int
	}{
		{
			label:         "first failure",
			path:          "A",
			body:          "1",
			status:        http.StatusInternalServerError,
			wantHitCounts: map[bool]int{false: 1},
			wantBody:      "1",
			wantStatus:    http.StatusInternalServerError,
		},
		{
			label:         "first success",
			path:          "A",
			body:          "2",
			status:        http.StatusOK,
			wantHitCounts: map[bool]int{false: 2},
			wantBody:      "2",
			wantStatus:    http.StatusOK,
		},
		{
			label:         "B is uncached",
			advanceTime:   10 * time.Second,
			path:          "B",
			body:          "3",
			status:        http.StatusForbidden,
			wantHitCounts: map[bool]int{false: 2},
			wantBody:      "3",
			wantStatus:    http.StatusForbidden,
		},
		{
			label: "A is cached",
			path:  "A",
			// These shouldn't matter, since we'll hit the cache.
			body:          "3",
			status:        http.StatusForbidden,
			wantHitCounts: map[bool]int{false: 2, true: 1},
			wantBody:      "2",
			wantStatus:    http.StatusOK,
		},
		{
			label: "cache expires",
			path:  "A",
			// with the ten seconds above, this should expire the 1 minute cache.
			advanceTime: 1 * time.Minute,
			body:        "4",
			// status is the zero value, but caching should still trigger.
			wantHitCounts: map[bool]int{false: 3, true: 1},
			wantBody:      "4",
			wantStatus:    http.StatusOK,
		},
		{
			label: "A is cached again",
			path:  "A",
			// 30 seconds is not enough time to expire the cache.
			advanceTime:   30 * time.Second,
			body:          "5",
			wantHitCounts: map[bool]int{false: 3, true: 2},
			wantBody:      "4",
			wantStatus:    http.StatusOK,
		},
		{
			label:  "bypassing the cache",
			path:   "A",
			body:   "6",
			bypass: true,
			// hitCounts should not be modified.
			wantHitCounts: map[bool]int{false: 3, true: 2},
			wantBody:      "6",
			wantStatus:    http.StatusOK,
		},
	}

	for _, test := range tests {
		s.FastForward(test.advanceTime)
		body = test.body
		status = test.status
		req, err := http.NewRequest("GET", ts.URL+"/"+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if test.bypass {
			req.Header.Set(config.BypassCacheAuthHeader, "yes")
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != test.wantStatus {
			t.Errorf("[%s] GET returned status %d, want %d", test.label, resp.StatusCode, test.wantStatus)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if string(body) != test.wantBody {
			t.Errorf("[%s] GET returned body %s, want %s", test.label, string(body), test.wantBody)
		}
		rows, err := view.RetrieveData(CacheResultCount.Name)
		if err != nil {
			t.Fatal(err)
		}
		hitCounts := make(map[bool]int)
		for _, row := range rows {
			// Tags[0] should always be the hit result (true or false). For
			// simplicity we assume this.
			source, err := strconv.ParseBool(row.Tags[0].Value)
			if err != nil {
				t.Fatal(err)
			}
			count := int(row.Data.(*view.CountData).Value)
			hitCounts[source] = count
		}
		if diff := cmp.Diff(test.wantHitCounts, hitCounts); diff != "" {
			t.Errorf("[%s] CacheResultCount diff (-want +got):\n%s", test.label, diff)
		}
	}
}
