// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package generics uses generics.
package generics

type Pair[A, B any] struct {
	V0 A
	V1 B
}

// NewPair returns a new Pair.
func NewPair[A, B any](a A, b B) Pair[A, B] {
	return Pair[A, B]{a, b}
}
