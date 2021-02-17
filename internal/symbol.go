// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

// SymbolSection is the documentation section where a symbol appears.
type SymbolSection string

const (
	SymbolSectionConstants SymbolSection = "Constants"
	SymbolSectionVariables SymbolSection = "Variables"
	SymbolSectionFunctions SymbolSection = "Functions"
	SymbolSectionTypes     SymbolSection = "Types"
)

// SymbolKind is the type of a symbol.
type SymbolKind string

const (
	SymbolKindConstant SymbolKind = "Constant"
	SymbolKindVariable SymbolKind = "Variable"
	SymbolKindFunction SymbolKind = "Function"
	SymbolKindType     SymbolKind = "Type"
	SymbolKindField    SymbolKind = "Field"
	SymbolKindMethod   SymbolKind = "Method"
)

// Symbol is an element in the package API. A symbol can be a constant,
// variable, function of type.
type Symbol struct {
	// Name is the name of the symbol.
	Name string

	// Synopsis is the one line description of the symbol as displayed
	// in the package documentation.
	Synopsis string

	// Section is the section that a symbol appears in.
	Section SymbolSection

	// Kind is the type of a symbol, which is either a constant, variable,
	// function, type, field or method.
	Kind SymbolKind

	// ParentName if name of the parent type if available, otherwise
	// the empty string. For example, the parent type for
	// net/http.FileServer is Handler.
	ParentName string

	// Children contain the child symbols for this symbol. This will
	// only be populated when the SymbolType is "Type". For example, the
	// children of net/http.Handler are FileServer, NotFoundHandler,
	// RedirectHandler, StripPrefix, and TimeoutHandler. Each child
	// symbol will have ParentName set to the Name of this type.
	Children []*Symbol

	// SinceVersion is the first version when the symbol was introduced.
	SinceVersion string

	// GOOS specifies the execution operating system where the symbol appears.
	GOOS string

	// GOARCH specifies the execution architecture where the symbol appears.
	GOARCH string
}
