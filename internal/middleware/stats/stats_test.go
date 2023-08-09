// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"encoding/json"
	"hash/fnv"
	"io"
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
	var afterFirstWrite, afterSleep, handlerStart time.Time
	ts := httptest.NewServer(Stats()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerStart = time.Now()
		ctx := r.Context()
		w.WriteHeader(code)
		set(ctx, "a", 1)
		w.Write(data[:10])
		afterFirstWrite = time.Now()
		set(ctx, "b", 2)
		time.Sleep(10 * time.Millisecond)
		afterSleep = time.Now()
		set(ctx, "a", 3)
		w.Write(data[10:])
	})))
	defer ts.Close()
	start := time.Now()
	res, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("failed with status %d", res.StatusCode)
	}
	gotData, err := io.ReadAll(res.Body)
	end := time.Now()
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
		Other: map[string]any{
			// JSON unmarshals all numbers into float64s.
			"a": []any{float64(1), float64(3)},
			"b": float64(2),
		},
	}
	diff := cmp.Diff(want, got, cmpopts.IgnoreFields(PageStats{}, "MillisToFirstByte", "MillisToLastByte"))
	if diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
	timeToFirstByteUpperBound := afterFirstWrite.Sub(start)
	if g := got.MillisToFirstByte; g > timeToFirstByteUpperBound.Milliseconds() {
		t.Errorf("MillisToFirstByte is %d, wanted <= %d", g, timeToFirstByteUpperBound)
	}
	timeToLastByteLowerBound := afterSleep.Sub(handlerStart)
	timeToLastByteUpperBound := end.Sub(start)
	if g := got.MillisToLastByte; g < timeToLastByteLowerBound.Milliseconds() || g > timeToLastByteUpperBound.Milliseconds() {
		t.Errorf("MillisToLastByte is %d, wanted >= %d and <= %d",
			g, timeToLastByteLowerBound.Milliseconds(), timeToLastByteUpperBound.Milliseconds())
	}
}
