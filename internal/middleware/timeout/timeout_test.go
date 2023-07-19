// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package timeout

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			http.Error(w, "bad", http.StatusInternalServerError)
			return
		default:
		}
		fmt.Fprintln(w, "Hello!")
	})

	mux := http.NewServeMux()
	mux.Handle("/A", Timeout(5*time.Second)(handler))
	mux.Handle("/B", Timeout(0)(handler))
	mux.Handle("/C", handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	tests := []struct {
		label      string
		url        string
		wantStatus int
	}{
		{
			label:      "normal timed request",
			url:        fmt.Sprintf("%s/A", ts.URL),
			wantStatus: http.StatusOK,
		},
		{
			label:      "timed-out request",
			url:        fmt.Sprintf("%s/B", ts.URL),
			wantStatus: http.StatusInternalServerError,
		},
		{
			label:      "request with no timeout",
			url:        fmt.Sprintf("%s/C", ts.URL),
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			resp, err := ts.Client().Get(test.url)
			if err != nil {
				t.Errorf("GET %s got error %v, want nil", test.url, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != test.wantStatus {
				t.Errorf("GET %s returned status %d, want %d", test.url, resp.StatusCode, test.wantStatus)
			}
		})
	}
}
