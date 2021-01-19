// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package has symbols for testing changes
// to the AST to improve the generated documentation.
// See TestRewriteDecl in ../linkify_test.go.
package rewrites

const OK = "ok"

const OKWithComment = "ok" // comment

const Long = "long"

const LongWithComment = "long" // comment

const (
	GroupOK              = "ok"
	GroupWithComment     = "ok" // comment
	GroupLong            = "long"
	GroupLongWithComment = "long" // comment
)

type FieldTag struct {
	F1 int `ok`
	F2 int `long`
	F3 int `long` // comment
}

type FieldTagFiltered struct {
	u int

	Name string `long` // comment
}

var CompositeOK = []int{1, 2}

var CompositeLong = []int{1, 2, 3}

var CompositeLongComment = []int{1, 2, 3} // comment
