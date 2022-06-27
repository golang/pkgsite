// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package comments exercises the Go 1.19 doc comment features.
// This refers to the standard library [encoding/json] package.
package comments

import (
	"github.com/google/go-cmp/cmp"
	safe "github.com/google/safehtml"
)

// F refers to function [G] and method [T.M].
// It also has three bullet points:
//   - one
//   - two
//   - three
//
// # Example
//
// Here is an example:
//
//	F()
func F() {
}

// G implements something according to [this link].
//
// [this link]: https://pkg.go.dev
func G() {
	_ = cmp.Diff("x", "y")
}

// A numbered list looks like
//  1. a
//  2. b
//
// This link refers to a package in the same
// module: [example.com/module/pkg].
type T struct{}

// M refers to [F].
// It also refers to packages [safe]
// and [cmp]
// and [golang/org/x/sync/errgroup]
// and to the symbols [safe.HTML]
// and [cmp.Diff]
// and [golang.org/x/sync/errgroup.Group].
func (T) M() safe.HTML {
	return safe.HTML{}
}
