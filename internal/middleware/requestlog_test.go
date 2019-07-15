// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/logging"
	"github.com/google/go-cmp/cmp"
)

func TestRequestLog(t *testing.T) {
	tests := []struct {
		label   string
		handler http.HandlerFunc
		want    fakeLog
	}{
		{
			label: "writes status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
			},
			want: fakeLog{Status: 400},
		},
		{
			label:   "translates 200s",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			want:    fakeLog{Status: 200},
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			lg := fakeLog{}
			mw := RequestLog(&lg)
			ts := httptest.NewServer(mw(test.handler))
			defer ts.Close()
			_, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("GET returned error %v", err)
			}
			if diff := cmp.Diff(test.want, lg); diff != "" {
				t.Errorf("mismatching log state (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeLog struct {
	Status int
}

func (l *fakeLog) Log(entry logging.Entry) {
	if entry.HTTPRequest != nil {
		l.Status = entry.HTTPRequest.Status
	}
}
