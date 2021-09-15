// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package search

import "fmt"

var Content = fmt.Sprintf(`// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated with go generate -run gen_query.go. DO NOT EDIT.

package search

// querySearchSymbol is used when the search query is only one word, with no dots.
// In this case, the word must match a symbol name and ranking is completely
// determined by the path_tokens.
%s

// querySearchPackageDotSymbol is used when the search query is one element
// containing a dot, where the first part is assumed to be the package name and
// the second the symbol name. For example, "sql.DB" or "sql.DB.Begin".
%s

// querySearchMultiWordExact is used when the search query is multiple elements.
%s
`,
	formatQuery("querySearchSymbol", SymbolQuery(SearchTypeSymbol)),
	formatQuery("querySearchPackageDotSymbol", SymbolQuery(SearchTypePackageDotSymbol)),
	formatQuery("querySearchMultiWordExact", SymbolQuery(SearchTypeMultiWordExact)))

func formatQuery(name, query string) string {
	return fmt.Sprintf("const %s = `%s`", name, query)
}
