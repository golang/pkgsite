// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
)

func TestParse(t *testing.T) {
	for _, test := range []struct {
		name, want string
		section    SectionType
	}{
		{
			name:    "sidenav",
			section: SidenavSection,
			want:    quoteSidenav,
		},
		{
			name:    "sidenav-mobile",
			section: SidenavMobileSection,
			want:    quoteSidenavMobile,
		},
		{
			name:    "body",
			section: BodySection,
			want:    quoteBody,
		},
	} {
		{
			t.Run(test.name, func(t *testing.T) {
				got, err := Parse(safehtml.HTMLEscaped(quoteDocHTML), test.section)
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got.String()); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			})
		}
	}
}
