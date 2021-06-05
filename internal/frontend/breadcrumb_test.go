// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/version"
)

func TestBreadcrumbPath(t *testing.T) {
	for _, test := range []struct {
		pkgPath, modPath, version string
		want                      breadcrumb
	}{
		{
			"example.com/blob/s3blob", "example.com", version.Latest,
			breadcrumb{
				Current: "s3blob",
				Links: []link{
					{"/example.com", "example.com"},
					{"/example.com/blob", "blob"},
				},
				CopyData: "example.com/blob/s3blob",
			},
		},
		{
			"example.com", "example.com", version.Latest,
			breadcrumb{
				Current:  "example.com",
				Links:    []link{},
				CopyData: "example.com",
			},
		},
		{
			"g/x/tools/go/a", "g/x/tools", version.Latest,
			breadcrumb{
				Current: "a",
				Links: []link{
					{"/g/x/tools", "g/x/tools"},
					{"/g/x/tools/go", "go"},
				},
				CopyData: "g/x/tools/go/a",
			},
		},
		{
			"golang.org/x/tools", "golang.org/x/tools", version.Latest,
			breadcrumb{
				Current:  "golang.org/x/tools",
				Links:    []link{},
				CopyData: "golang.org/x/tools",
			},
		},
		{
			// Special case: stdlib package.
			"encoding/json", "std", version.Latest,
			breadcrumb{
				Current:  "json",
				Links:    []link{{"/encoding", "encoding"}},
				CopyData: "encoding/json",
			},
		},
		{
			// Special case: stdlib package.
			"encoding/json", "std", "go1.15",
			breadcrumb{
				Current:  "json",
				Links:    []link{{"/encoding@go1.15", "encoding"}},
				CopyData: "encoding/json",
			},
		},
		{
			// Special case: stdlib module.
			"std", "std", version.Latest,
			breadcrumb{
				Current: "Standard library",
				Links:   nil,
			},
		},
		{
			"example.com/blob/s3blob", "example.com", "v1",
			breadcrumb{
				Current: "s3blob",
				Links: []link{
					{"/example.com@v1", "example.com"},
					{"/example.com/blob@v1", "blob"},
				},
				CopyData: "example.com/blob/s3blob",
			},
		},
	} {
		t.Run(fmt.Sprintf("%s-%s-%s", test.pkgPath, test.modPath, test.version), func(t *testing.T) {
			got := breadcrumbPath(test.pkgPath, test.modPath, test.version)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
