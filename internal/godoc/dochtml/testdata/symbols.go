// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package symbols is designed to test symbols from a docPackage.
package symbols

// const
const C = 1

// const iota
const (
	AA = iota + 1
	_
	BB
	CC
)

type Num int

const (
	DD Num = iota
	_
	EE
	FF
)

// var
var V = 2

// Multiple variables on the same line.
var A, B string

// func
func F() {}

// type
type T int

// typeConstant
const CT T = 3

// typeVariable
var VT T

// multi-line var
var (
	ErrA = errors.New("error A")
	ErrB = errors.New("error B")
)

// typeFunc
func TF() T { return T(0) }

// method
// BUG(uid): this verifies that notes are rendered
func (T) M() {}

type S1 struct {
	F int // field
}

type S2 struct {
	S1 // embedded struct; should have an id
	G  int
}

type I1 interface {
	M1()
}

type I2 interface {
	I1 // embedded interface; should not have an id
	M2()
}

type (
	A int
	B bool
)
