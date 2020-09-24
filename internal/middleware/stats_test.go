// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"encoding/json"
	"hash/fnv"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestStats(t *testing.T) {
	data := []byte("this is the data we are going to serve")
	const code = 218
	ts := httptest.NewServer(Stats()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Write(data[:10])
		time.Sleep(500 * time.Millisecond)
		w.Write(data[10:])
	})))
	defer ts.Close()
	res, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("failed with status %d", res.StatusCode)
	}
	gotData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got PageStats
	if err := json.Unmarshal(gotData, &got); err != nil {
		t.Fatal(err)
	}

	h := fnv.New64a()
	h.Write(data)
	want := PageStats{
		StatusCode: code,
		Size:       len(data),
		Hash:       h.Sum64(),
	}
	diff := cmp.Diff(want, got, cmpopts.IgnoreFields(PageStats{}, "MillisToFirstByte", "MillisToLastByte"))
	if diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
	const tolerance = 50 // 50 ms of tolerance for time measurements
	if g := got.MillisToFirstByte; !within(g, 0, tolerance) {
		t.Errorf("MillisToFirstByte is %d, wanted 0 - %d", g, tolerance)
	}
	if g := got.MillisToLastByte; !within(g, 500, tolerance) {
		t.Errorf("MillisToLastByte is %d, wanted 500 +/- %d", g, tolerance)
	}
}

func within(got, want, tolerance int64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}
