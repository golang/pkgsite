// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package p is for testing godoc.Render. There are a lot
// of other things to say, but that's the gist of it.
//
//
// Links
//
// - pkg.go.dev, https://pkg.go.dev
package p

import (
	"fmt"
	"time"
)

// const
const C = 1

// var
var V = unexp()

// exported func
func F(t time.Time) {
	fmt.Println(t)
}

// unexported func
func unexp() {
}

// type
type T int

// typeConstant
const CT T = 3

// typeVariable
var VT T

// typeFunc
func TF() T {
	return T(0)
}

// method
// BUG(uid): this verifies that notes are rendered
func (T) M() {}

// unexported method
func (T) m() {}

type S1 struct {
	F int // field
}

type us struct {
	G bool
	u int
}
type S2 struct {
	S1 // embedded struct
	us // embedded, unexported struct
	H  int
}

// I is an interface.
type I interface {
	M1()
}
