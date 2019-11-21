// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	for _, test := range []struct {
		name   string
		latest latestFunc
		in     string
		want   string
	}{
		{
			name:   "package version is not latest",
			latest: func(context.Context, string, string) string { return "v1.2.3" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$"
					 data-version="v1.0.0" data-mpath="p1/p2" data-ppath="p1/p2/p3">
                    <span>Latest</span>
                    <a href="p1/p2@$$GODISCOVERY_LATESTVERSION$$/p3">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge DetailsHeader-goToLatest"
					 data-version="v1.0.0" data-mpath="p1/p2" data-ppath="p1/p2/p3">
                    <span>Latest</span>
                    <a href="p1/p2@v1.2.3/p3">Go to latest</a>
                </div>`,
		},
		{
			name:   "package version is latest",
			latest: func(context.Context, string, string) string { return "v1.2.3" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="p1/p2/p3">
                    <span>Latest</span>
                    <a href="p1/p2@$$GODISCOVERY_LATESTVERSION$$/p3">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge DetailsHeader-latest"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="p1/p2/p3">
                    <span>Latest</span>
                    <a href="p1/p2@v1.2.3/p3">Go to latest</a>
                </div>`,
		},
		{
			name:   "module version is not latest",
			latest: func(context.Context, string, string) string { return "v1.2.3" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$"
					 data-version="v1.0.0" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@$$GODISCOVERY_LATESTVERSION$$">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge DetailsHeader-goToLatest"
					 data-version="v1.0.0" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@v1.2.3">Go to latest</a>
                </div>`,
		},
		{
			name:   "module version is latest",
			latest: func(context.Context, string, string) string { return "v1.2.3" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@$$GODISCOVERY_LATESTVERSION$$">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge DetailsHeader-latest"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@v1.2.3">Go to latest</a>
                </div>`,
		},
		{
			name:   "latest func returns empty string",
			latest: func(context.Context, string, string) string { return "" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@$$GODISCOVERY_LATESTVERSION$$">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge DetailsHeader-unknown"
					 data-version="v1.2.3" data-mpath="p1/p2" data-ppath="">
                    <span>Latest</span>
                    <a href="mod/p1/p2@">Go to latest</a>
                </div>`,
		},
		{
			name:   "no regexp match",
			latest: func(context.Context, string, string) string { return "v1.2.3" },
			in: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$">
                    <span>Latest</span>
                    <a href="mod/p1/p2@$$GODISCOVERY_LATESTVERSION$$">Go to latest</a>
                </div>`,
			want: `
                <div class="DetailsHeader-badge $$GODISCOVERY_LATESTCLASS$$">
                    <span>Latest</span>
                    <a href="mod/p1/p2@$$GODISCOVERY_LATESTVERSION$$">Go to latest</a>
                </div>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, test.in)
			})
			ts := httptest.NewServer(LatestVersion(test.latest)(handler))
			defer ts.Close()
			resp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if string(got) != test.want {
				t.Errorf("\ngot  %s\nwant %s", got, test.want)
			}
		})
	}
}
