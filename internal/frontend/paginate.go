// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"math"
)

// pagination holds information related to pagination. It is intended to be
// embedded in a view model struct.
type pagination struct {
	Pages       []int
	PageCount   int
	TotalCount  int
	ResultCount int
	Offset      int
	PerPage     int
	Page        int
	PrevPage    int
	NextPage    int
}

func newPagination(page, resultCount, totalCount, limit int) pagination {
	return pagination{
		Pages:       pagesToLink(page, numPages(limit, totalCount), defaultNumPagesToLink),
		PageCount:   numPages(limit, totalCount),
		TotalCount:  totalCount,
		ResultCount: resultCount,
		Offset:      offset(page, limit),
		PerPage:     limit,
		Page:        page,
		PrevPage:    prev(page),
		NextPage:    next(page, limit, totalCount),
	}
}

const defaultNumPagesToLink = 7

// pagesToLink returns the page numbers that will be displayed. Given a
// page, it returns a slice containing numPagesToLink integers in ascending
// order and optimizes for page to be in the middle of that range. The max
// value of an integer in the return slice will be less than numPages.
func pagesToLink(page, numPages, numPagesToLink int) []int {
	var pages []int
	start := page - (numPagesToLink / 2)
	if (numPages - start) < numPagesToLink {
		start = numPages - numPagesToLink + 1
	}
	if start < 1 {
		start = 1
	}

	for i := start; (i < start+numPagesToLink) && (i <= numPages); i++ {
		pages = append(pages, i)
	}
	return pages
}

// numPages is the total number of pages needed to display all the results,
// given the specified limit.
func numPages(limit, totalCount int) int {
	return int(math.Ceil(float64(totalCount) / float64(limit)))
}

func offset(page, limit int) int {
	if page < 2 {
		return 0
	}
	return (page - 1) * limit
}

func prev(page int) int {
	if page < 2 {
		return 0
	}
	return page - 1
}

func next(page, limit, perPage int) int {
	if page >= numPages(limit, perPage) {
		return 0
	}
	return page + 1
}
