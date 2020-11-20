// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPagination(t *testing.T) {
	for _, test := range []struct {
		page, numResults, wantNumPages, wantOffset, wantPrev, wantNext int
		name                                                           string
	}{
		{
			name:         "single page of results with numResults below limit",
			page:         1,
			numResults:   7,
			wantNumPages: 1,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     0,
		},
		{
			name:         "single page of results with numResults exactly limit",
			page:         1,
			numResults:   10,
			wantNumPages: 1,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     0,
		},
		{
			name:         "first page of results for total of 5 pages",
			page:         1,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     2,
		},
		{
			name:         "second page of results for total of 5 pages",
			page:         2,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   10,
			wantPrev:     1,
			wantNext:     3,
		},
		{
			name:         "last page of results for total of 5 pages",
			page:         5,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   40,
			wantPrev:     4,
			wantNext:     0,
		},
		{
			name:         "page out of range",
			page:         8,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   70,
			wantPrev:     7,
			wantNext:     0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testLimit := 10
			if got := numPages(testLimit, test.numResults); got != test.wantNumPages {
				t.Errorf("numPages(%d, %d) = %d; want = %d",
					testLimit, test.numResults, got, test.wantNumPages)
			}
			if got := offset(test.page, testLimit); got != test.wantOffset {
				t.Errorf("offset(%d, %d) = %d; want = %d",
					test.page, testLimit, got, test.wantOffset)
			}
			if got := prev(test.page); got != test.wantPrev {
				t.Errorf("prev(%d) = %d; want = %d", test.page, got, test.wantPrev)
			}
			if got := next(test.page, testLimit, test.numResults); got != test.wantNext {
				t.Errorf("next(%d, %d, %d) = %d; want = %d",
					test.page, testLimit, test.numResults, got, test.wantNext)
			}
		})
	}
}

func TestPagesToDisplay(t *testing.T) {
	for _, test := range []struct {
		name                         string
		page, numPages, numToDisplay int
		wantPages                    []int
	}{
		{
			name:         "page 1 of 10 - first in range",
			page:         1,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{1, 2, 3, 4, 5},
		},
		{
			name:         "page 3 of 10 - last in range to include 1 in wantPages ",
			page:         3,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{1, 2, 3, 4, 5},
		},
		{
			name:         "page 4 of 10 - first in range to not include 1 in wantPages",
			page:         4,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{2, 3, 4, 5, 6},
		},
		{
			name:         "page 7 of 10 - page in the middle",
			page:         7,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{5, 6, 7, 8, 9},
		},
		{
			name:         "page 8 of 10- first in range to include page 10",
			page:         8,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{6, 7, 8, 9, 10},
		},
		{
			name:         "page 10 of 10 - last page in range",
			page:         10,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{6, 7, 8, 9, 10},
		},
		{
			name:         "page 1 of 11, displaying 4 pages - first in range",
			page:         1,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{1, 2, 3, 4},
		},
		{
			name:         "page 3 of 11, display 4 pages - last in range to include 1 in wantPages ",
			page:         3,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{1, 2, 3, 4},
		},
		{
			name:         "page 4 of 11, displaying 4 pages - first in range to not include 1 in wantPages",
			page:         4,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{2, 3, 4, 5},
		},
		{
			name:         "page 7 of 11, displaying 4 pages - page in the middle",
			page:         7,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{5, 6, 7, 8},
		},
		{
			name:         "page 8 of 11, displaying 4 pages",
			page:         8,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{6, 7, 8, 9},
		},
		{
			name:         "page 10 of 11, displaying 4 pages - second to last page in range",
			page:         10,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{8, 9, 10, 11},
		},
		{
			name:         "page 4 of 6, displays all pages",
			page:         4,
			numPages:     6,
			numToDisplay: 7,
			wantPages:    []int{1, 2, 3, 4, 5, 6},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := pagesToLink(test.page, test.numPages, test.numToDisplay)
			if diff := cmp.Diff(got, test.wantPages); diff != "" {
				t.Errorf("pagesToLink(%d, %d, %d) = %v; want = %v", test.page, test.numPages, test.numToDisplay, got, test.wantPages)
			}
		})
	}
}
