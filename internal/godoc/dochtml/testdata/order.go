// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package order exercises natural sorting of symbols.
package order

// Uint32x16 represents a 128-bit unsigned integer.
type Uint32x16 struct {
	data [16]uint32
}

// AsUint8x64 converts a Uint32x16 to a Uint8x64.
func (u Uint32x16) AsUint8x64() Uint8x64 {
	return Uint8x64{}
}

// AsUint64x8 converts a Uint32x16 to a Uint64x8.
func (u Uint32x16) AsUint64x8() Uint8x64 {
	return Uint64x8{}
}

// Uint8x64 represents a 128-bit unsigned integer.
type Uint8x64 struct {
	data [64]uint8
}

// AsUint32x16 converts a Uint8x64 to a Uint32x16.
func (u Uint8x64) AsUint32x16() Uint32x16 {
	return Uint32x16{}
}

// AsUint64x8 converts a Uint8x64 to a Uint64x8.
func (u Uint8x64) AsUint64x8() Uint64x8 {
	return Uint64x8{}
}

// Uint64x8 represents a 128-bit unsigned integer.
type Uint64x8 struct {
	data [8]uint64
}

// AsUint8x64 converts a Uint64x8 to a Uint8x64.
func (u Uint64x8) AsUint8x64() Uint8x64 {
	return Uint8x64{}
}

// AsUint32x16 converts a Uint64x8 to a Uint32x16.
func (u Uint64x8) AsUint32x16() Uint32x16 {
	return Uint32x16{}
}

func ExampleUint64x8_AsUint32x16() {}

func ExampleUint64x8_AsUint8x64() {}
