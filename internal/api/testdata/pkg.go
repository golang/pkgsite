// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pkg has every form of declaration.
//
// # Links
//
//   - pkgsite repo, https://go.googlesource.com/pkgsite
//   - Play with Go, https://play-with-go.dev
package pkg

// C is a shorthand for 1.
const C = 1

// No exported name; should not appear.
const (
	a = 1
	b = 2
	c = 3
)

// V is a variable.
var V = 2

// F is a function.
func F() {}

// Several constants.
const (
	X = 1
	Y = 2
)

// CT is a typed constant.
// They appear after their type.
const CT T = 3

// TF is a constructor for T.
func TF() T { return T(0) }

// M is a method of T.
// BUG(xxx): this verifies that notes are rendered.
func (T) M() {}

// T is a type.
type T int

// S1 is a struct.
type S1 struct {
	F int // field
}

// S2 is another struct.
type S2 struct {
	S1
	G int
}

// I1 is an interface.
type I1 interface {
	M1()
}

type I2 interface {
	I1
	M2()
}

type (
	A int
	B bool
)

// Add adds 1 to x.
func Add(x int) int {
	return x + 1
}
