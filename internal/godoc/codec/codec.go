// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package codec implements the general-purpose part of an encoder for Go
// values. It relies on code generation rather than reflection so it is
// significantly faster than reflection-based encoders like gob. It also
// preserves sharing among struct pointers (but not other forms of sharing, like
// sub-slices). These features are sufficient for encoding the structures of the
// go/ast package, which is its sole purpose.
package codec

import (
	"encoding/binary"
	"math"
)

// An Encoder encodes Go values into a sequence of bytes.
type Encoder struct {
	buf []byte
}

// NewEncoder returns an Encoder.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// Bytes returns the encoded byte slice.
func (e *Encoder) Bytes() []byte {
	return e.buf
}

// A Decoder decodes a Go value encoded by an Encoder.
type Decoder struct {
	buf []byte
	i   int
}

// NewDecoder returns a Decoder for the given bytes.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{buf: data, i: 0}
}

//////////////// Low-level I/O

func (e *Encoder) writeByte(b byte) {
	e.buf = append(e.buf, b)
}

func (d *Decoder) readByte() byte {
	b := d.curByte()
	d.i++
	return b
}

// curByte returns the next byte to be read
// without actually consuming it.
func (d *Decoder) curByte() byte {
	return d.buf[d.i]
}

func (e *Encoder) writeBytes(b []byte) {
	e.buf = append(e.buf, b...)
}

// readBytes reads and returns the given number of bytes.
// It fails if there are not enough bytes in the input.
func (d *Decoder) readBytes(n int) []byte {
	d.i += n
	return d.buf[d.i-n : d.i]
}

func (e *Encoder) writeString(s string) {
	e.buf = append(e.buf, s...)
}

func (d *Decoder) readString(len int) string {
	return string(d.readBytes(len))
}

func (e *Encoder) writeUint32(u uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], u)
	e.writeBytes(buf[:])
}

func (d *Decoder) readUint32() uint32 {
	return binary.LittleEndian.Uint32(d.readBytes(4))
}

func (e *Encoder) writeUint64(u uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], u)
	e.writeBytes(buf[:])
}

func (d *Decoder) readUint64() uint64 {
	return binary.LittleEndian.Uint64(d.readBytes(8))
}

//////////////// Encoding Scheme

// Every encoded value begins with a single byte that describes what (if
// anything) follows. There is enough information to skip over the value, since
// the decoder must be able to do that if it encounters a struct field it
// doesn't know.
//
// Most of the values of that initial byte can be devoted to small unsigned
// integers. For example, the number 17 is represented by the single byte 17.
// Only five byte values have special meaning.
//
// nBytes (255) indicates that an unsigned integer N is encoded next,
// followed by N bytes of data. This is used to represent strings and byte
// slices, as well numbers bigger than can fit into the initial byte. For example,
// the string "hi" is represented as:
//   nBytes 2 'h' 'i'
//
// Unsigned integers that can't fit into the initial byte are encoded as byte
// sequences of length 4 or 8, holding little-endian uint32 or uint64 values. We
// use uint32s where possible to save space. We could have saved more space by
// also considering 16-byte numbers, or using a variable-length encoding like
// varints or gob's representation, but it didn't seem worth the additional
// complexity.
//
// TODO: describe nValues, ref, start and end in later CLs.

const (
	nBytesCode  = 255 - iota // uint n follows, then n bytes
	nValuesCode              // uint n follows, then n values
	refCode                  // uint n follows, referring to a previous value
	startCode                // start of a value of indeterminate length
	endCode                  // end of a value that began with with start
	// Bytes less than endCode represent themselves.
)

// EncodeUint encodes a uint64.
func (e *Encoder) EncodeUint(u uint64) {
	switch {
	case u < endCode:
		// u fits into the initial byte.
		e.writeByte(byte(u))
	case u <= math.MaxUint32:
		// Encode as a sequence of 4 bytes, the little-endian representation of
		// a uint32.
		e.writeByte(nBytesCode)
		e.writeByte(4)
		e.writeUint32(uint32(u))
	default:
		// Encode as a sequence of 8 bytes, the little-endian representation of
		// a uint64.
		e.writeByte(nBytesCode)
		e.writeByte(8)
		e.writeUint64(u)
	}
}
