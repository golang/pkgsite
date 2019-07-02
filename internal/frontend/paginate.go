// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
)

// pagination holds information related to pagination. It is intended to be
// embedded in a view model struct.
type pagination struct {
	BaseURL     string
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

func (p pagination) PageURL(page int) string {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		log.Printf("BUG: error parsing page base URL: %v", err)
	}
	newQuery := u.Query()
	newQuery.Set("page", strconv.Itoa(page))
	u.RawQuery = newQuery.Encode()
	return u.String()
}

func newPagination(params paginationParams, resultCount, totalCount int) pagination {
	return pagination{
		BaseURL:     params.baseURL,
		Pages:       pagesToLink(params.page, numPages(params.limit, totalCount), defaultNumPagesToLink),
		PageCount:   numPages(params.limit, totalCount),
		TotalCount:  totalCount,
		ResultCount: resultCount,
		Offset:      params.offset(),
		PerPage:     params.limit,
		Page:        params.page,
		PrevPage:    prev(params.page),
		NextPage:    next(params.page, params.limit, totalCount),
	}
}

// paginationParams holds pagination parameters extracted from the request.
type paginationParams struct {
	baseURL     string
	page, limit int
}

func (p paginationParams) offset() int {
	return offset(p.page, p.limit)
}

// newPaginationParams extracts pagination params from the request.
func newPaginationParams(r *http.Request, defaultLimit int) paginationParams {
	positiveParam := func(key string, dflt int) (val int) {
		var err error
		if a := r.FormValue(key); a != "" {
			val, err = strconv.Atoi(a)
			if err != nil {
				log.Printf("strconv.Atoi(%q) for page: %v", a, err)
			}
		}
		if val < 1 {
			val = dflt
		}
		return
	}
	return paginationParams{
		baseURL: r.URL.String(),
		page:    positiveParam("page", 1),
		limit:   positiveParam("limit", defaultLimit),
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
	return (totalCount + limit - 1) / limit
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
