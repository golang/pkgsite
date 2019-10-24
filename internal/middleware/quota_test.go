// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/stats/view"
)

func TestQuota(t *testing.T) {
	mw := Quota(QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 1})
	var npass int
	h := func(w http.ResponseWriter, r *http.Request) {
		npass++
	}
	ts := httptest.NewServer(mw(http.HandlerFunc(h)))
	defer ts.Close()
	c := ts.Client()
	view.Register(QuotaResultCount)
	defer view.Unregister(QuotaResultCount)

	check := func(msg string, nwant int) {
		npass = 0
		for i := 0; i < 5; i++ {
			req, err := http.NewRequest("GET", ts.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Add("X-Forwarded-For", "1.2.3.4, and more")
			res, err := c.Do(req)
			if err != nil {
				t.Fatalf("%s: %v", msg, err)
			}
			res.Body.Close()
			want := http.StatusOK
			if i >= nwant {
				want = http.StatusTooManyRequests
			}
			if got := res.StatusCode; got != want {
				t.Errorf("%s, #%d: got %d, want %d", msg, i, got, want)
			}
		}
		if npass != nwant {
			t.Errorf("%s: got %d requests to pass, want %d", msg, npass, nwant)
		}
	}

	// When making multiple requests in quick succession from the same IP,
	// only the first two get through; the rest are blocked.
	check("before", 2)
	// After a second (and a bit more), we should have one token back, meaning
	// we can serve one request.
	time.Sleep(1100 * time.Millisecond)
	check("after", 1)

	// Check the metric.
	got := collectViewData(t)
	want := map[bool]int{true: 7, false: 3} // only 3 requests of the ten we sent get through.
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestQuotaRecordOnly(t *testing.T) {
	// Like TestQuota, but with in RecordOnly mode nothing is actually blocked.
	mw := Quota(QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 1, RecordOnly: true})
	npass := 0
	h := func(w http.ResponseWriter, r *http.Request) {
		npass++
	}
	ts := httptest.NewServer(mw(http.HandlerFunc(h)))
	defer ts.Close()
	c := ts.Client()
	view.Register(QuotaResultCount)
	defer view.Unregister(QuotaResultCount)

	const nreq = 100
	for i := 0; i < nreq; i++ {
		req, err := http.NewRequest("GET", ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Add("X-Forwarded-For", "1.2.3.4, and more")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	if npass != nreq {
		t.Errorf("%d passed, want %d", npass, nreq)
	}
	got := collectViewData(t)
	want := map[bool]int{true: nreq - 2, false: 2} // record as if blocking occurred
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestQuotaBadKey(t *testing.T) {
	// Verify that invalid IP addresses are not blocked.
	mw := Quota(QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 1, RecordOnly: true})
	npass := 0
	h := func(w http.ResponseWriter, r *http.Request) {
		npass++
	}
	ts := httptest.NewServer(mw(http.HandlerFunc(h)))
	defer ts.Close()
	c := ts.Client()
	view.Register(QuotaResultCount)
	defer view.Unregister(QuotaResultCount)

	const nreq = 100
	for i := 0; i < nreq; i++ {
		req, err := http.NewRequest("GET", ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Add("X-Forwarded-For", "not.a.valid.ip, and more")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	if npass != nreq {
		t.Errorf("%d passed, want %d", npass, nreq)
	}
	got := collectViewData(t)
	want := map[bool]int{false: nreq} // no blocking occurred
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func collectViewData(t *testing.T) map[bool]int {
	m := map[bool]int{}
	rows, err := view.RetrieveData(QuotaResultCount.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		blocked, err := strconv.ParseBool(row.Tags[0].Value)
		if err != nil {
			t.Fatalf("collectViewData: %v", err)
		}
		count := int(row.Data.(*view.CountData).Value)
		m[blocked] = count
	}
	return m
}

func TestIPKey(t *testing.T) {
	for _, test := range []struct {
		in   string
		want interface{}
	}{
		{"", ""},
		{"1.2.3", ""},
		{"128.197.17.3", "128.197.17.0"},
		{"  128.197.17.3, foo  ", "128.197.17.0"},
		{"2001:db8::ff00:42:8329", "2001:db8::ff00:42:8300"},
	} {
		got := ipKey(test.in)
		if got != test.want {
			t.Errorf("%q: got %v, want %v", test.in, got, test.want)
		}
	}
}
