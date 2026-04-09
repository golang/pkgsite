// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadRouteInfo(t *testing.T) {
	for _, test := range []struct {
		name    string
		data    string
		want    []*RouteInfo
		wantErr bool
	}{
		{
			name: "correct",
			data: `
//api:route /v1/package/{path}
//api:desc Get package metadata.
//api:params path, version, module
//api:response Package
//api:route /v1/module/{path}
//api:desc Get module metadata.
//api:params path, version
//api:response Module
`,
			want: []*RouteInfo{
				{
					Route:    "/v1/package/{path}",
					Desc:     "Get package metadata.",
					Params:   "path, version, module",
					Response: "Package",
				},
				{
					Route:    "/v1/module/{path}",
					Desc:     "Get module metadata.",
					Params:   "path, version",
					Response: "Module",
				},
			},
		},
		{
			name: "missing field",
			data: `
//api:route /v1/package/{path}
//api:desc Get package metadata.
//api:response Package
`,
			wantErr: true,
		},
		{
			name:    "no routes",
			data:    "package api\n\n// some other comment",
			wantErr: true,
		},
		{
			name: "empty value",
			data: `
//api:route /v1/package/{path}
//api:desc
`,
			wantErr: true,
		},
		{
			name: "unknown key",
			data: `
//api:route /v1/package/{path}
//api:unknown something
`,
			wantErr: true,
		},
		{
			name: "duplicate route",
			data: `
//api:route /v1/package/{path}
//api:route /v1/other
`,
			wantErr: true,
		},
		{
			name: "duplicate desc",
			data: `
//api:route /v1/package/{path}
//api:desc Get package metadata.
//api:desc Something else.
`,
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := readRouteInfo([]byte(test.data))
			if (err != nil) != test.wantErr {
				t.Errorf("ReadRouteInfo() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if !test.wantErr {
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("ReadRouteInfo() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestRouteInfos(t *testing.T) {
	// Just check that there are no errors.
	if _, err := RouteInfos(); err != nil {
		t.Fatalf("RouteInfos failed: %v", err)
	}
}
