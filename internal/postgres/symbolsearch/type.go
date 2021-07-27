// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbolsearch

import (
	"strings"
)

// ParseInputType parses the search query input and returns the InputType. The
// InputType determines which symbol search query will be run.
func ParseInputType(q string) InputType {
	q = strings.TrimSpace(q)
	if strings.ContainsAny(q, " \t\n") {
		return InputTypeMultiWord
	}
	switch strings.Count(q, ".") {
	case 0:
		return InputTypeNoDot
	case 1:
		return InputTypeOneDot
	case 2:
		return InputTypeTwoDots
	default:
		return InputTypeNoMatch
	}
}

// InputType is the type determined for the search query input.
type InputType int

const (
	// InputTypeNoMatch indicates that there is no situation where we will get
	// results for this search input.
	InputTypeNoMatch InputType = iota

	// InputTypeNoDot indicates that the query type is <symbol>.
	//
	// If the search input contains only 1 word with no dots, it must be the
	// symbol name.
	InputTypeNoDot

	// InputTypeOneDot indicates that the query type is <package>.<symbol> or
	// <type>.<fieldOrMethod>.
	//
	// If the search input contains only 1 word split by 1 dot, the search must
	// either be for <package>.<symbol> or <type>.<methodOrFieldName>.
	InputTypeOneDot

	// InputTypeTwoDots indicates that the query type is
	// <package>.<type>.<fieldOrMethod>.
	//
	// If the search input contains only 1 word split by 1 dot, the search must
	// be for <package>.<type>.<methodOrFieldName>.
	// TODO(golang/go#44142): This could also be a search for
	// <package-path>.<symbol>, but that case is not currently handled.
	InputTypeTwoDots

	// InputTypeMultiWord indicates that the query has multiple words.
	InputTypeMultiWord
)

// SearchType is the type of search that will be performed, based on the input
// type.
type SearchType int

const (
	// SearchTypeSymbol is used for InputTypeNoDot (input is <symbol>) or
	// InputTypeOneDot (input is <type>.<fieldOrMethod>).
	SearchTypeSymbol SearchType = iota

	// SearchTypePackageDotSymbol is used for
	// InputTypeNoDot (input is <package>.<symbol>) or
	// InputTypeTwoDots (input is <package>.<type>.<fieldOrMethod>).
	SearchTypePackageDotSymbol

	// SearchTypeMultiWord is used for InputTypeMultiWord.
	SearchTypeMultiWord
)
