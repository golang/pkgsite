// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains syntax-only representations of types.

package api

import (
	"fmt"
	"go/ast"
)

// changeKind is the kind of change between two API elements.
type changeKind int

const (
	changeOther          changeKind = iota // no change or compatible change
	changeCallCompatible                   // technically a breaking change, but calls will compile
	changeBreaking
)

func (c changeKind) String() string {
	switch c {
	case changeOther:
		return "other"
	case changeCallCompatible:
		return "call-compatible"
	case changeBreaking:
		return "breaking"
	default:
		return fmt.Sprintf("changeKind(%d)", c)
	}
}

// syntaxType is a Go type extracted from syntax.
type syntaxType interface {
	// change reports the most significant change between the receiver,
	// which is the older version, and the argument. A breaking change
	// is considered more significant than a call-compatible change, which
	// is more significant than an "other" change.
	change(newType syntaxType) changeKind
}

// simpleType is a type represented only by a type string.
// The key property of a simpleType is that any change is a breaking
// one. It is used not only for basic types like int and string, but
// also for slices, arrays, maps, and channels. (Technically you can
// drop a direction on a channel compatibly, but that's rare enough so
// that we don't bother with it.)
type simpleType struct {
	typeString string
}

func newSimpleType(e ast.Expr) *simpleType {
	return &simpleType{typeString: typeString(e)}
}

// change returns the kind of change from old to new.
func (old *simpleType) change(newType syntaxType) changeKind {
	if n, ok := newType.(*simpleType); ok && old.typeString == n.typeString {
		return changeOther
	}
	return changeBreaking
}
