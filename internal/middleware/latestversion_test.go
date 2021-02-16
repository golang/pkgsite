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

	"golang.org/x/pkgsite/internal"
)

func TestLatestMajorVersion(t *testing.T) {
	for _, test := range []struct {
		name        string
		latest      internal.LatestInfo
		modulePaths []string
		in          string
		want        string
	}{
		{
			name:   "module path is not at latest",
			latest: internal.LatestInfo{MajorModulePath: "foo.com/bar/v3", MajorUnitPath: "foo.com/bar/v3"},
			modulePaths: []string{
				"foo.com/bar",
				"foo.com/bar/v2",
				"foo.com/bar/v3",
			},
			in: `
				<div class="DetailsHeader-banner$$GODISCOVERY_LATESTMAJORCLASS$$">
					 data-version="v1.0.0" data-mpath="foo.com/bar" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/$$GODISCOVERY_LATESTMAJORVERSIONURL$$">$$GODISCOVERY_LATESTMAJORVERSION$$</a>.
					</p>
				</div>`,
			want: `
				<div class="DetailsHeader-banner">
					 data-version="v1.0.0" data-mpath="foo.com/bar" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/foo.com/bar/v3">v3</a>.
					</p>
				</div>`,
		},
		{
			name:   "module path is at latest",
			latest: internal.LatestInfo{MajorModulePath: "foo.com/bar/v3", MajorUnitPath: "foo.com/bar/v3"},
			modulePaths: []string{
				"foo.com/bar",
				"foo.com/bar/v2",
				"foo.com/bar/v3",
			},
			in: `
				<div class="DetailsHeader-banner$$GODISCOVERY_LATESTMAJORCLASS$$">
					 data-version="v3.0.0" data-mpath="foo.com/bar/v3" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/$$GODISCOVERY_LATESTMAJORVERSIONURL$$">$$GODISCOVERY_LATESTMAJORVERSION$$</a>.
					</p>
				</div>`,
			want: `
				<div class="DetailsHeader-banner DetailsHeader-banner--latest">
					 data-version="v3.0.0" data-mpath="foo.com/bar/v3" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/foo.com/bar/v3">v3</a>.
					</p>
				</div>`,
		},
		{
			name:   "full path is not at the latest",
			latest: internal.LatestInfo{MajorModulePath: "foo.com/bar/v3", MajorUnitPath: "foo.com/bar/v3/far"},
			modulePaths: []string{
				"foo.com/bar",
				"foo.com/bar/v2",
				"foo.com/bar/v3",
			},
			in: `
				<div class="DetailsHeader-banner$$GODISCOVERY_LATESTMAJORCLASS$$">
					data-version="v1.0.0" data-mpath="foo.com/bar" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/$$GODISCOVERY_LATESTMAJORVERSIONURL$$">$$GODISCOVERY_LATESTMAJORVERSION$$</a>.
					</p>
				</div>`,
			want: `
				<div class="DetailsHeader-banner">
					data-version="v1.0.0" data-mpath="foo.com/bar" data-ppath="foo.com/bar/far" data-pagetype="pkg">
					<p>
						The highest tagged major version is <a href="/foo.com/bar/v3/far">v3</a>.
					</p>
				</div>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, test.in)
			})
			lfunc := func(context.Context, string, string) internal.LatestInfo { return test.latest }
			ts := httptest.NewServer(LatestVersions(lfunc)(handler))
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
