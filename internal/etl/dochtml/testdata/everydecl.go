// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package everydecl has every form of declaration known to dochtml.
// It is designed to test that the generated HTML has the right id and data-kind
// attributes.
package everydecl

// const
const C = 1

// var
var V = 2

// func
func F() {}

// type
type T int

// typeConstant
const CT T = 3

// typeVariable
var VT T

// typeFunc
func TF() T { return T(0) }

// method
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
