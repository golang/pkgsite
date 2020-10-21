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
	"fmt"
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

func (d *Decoder) fail(err error) {
	panic(err)
}

func (d *Decoder) failf(format string, args ...interface{}) {
	d.fail(fmt.Errorf(format, args...))
}

func (d *Decoder) badcode(c byte) {
	d.failf("bad code: %d", c)
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

// DecodeUint decodes a uint64.
func (d *Decoder) DecodeUint() uint64 {
	b := d.readByte()
	switch {
	case b < endCode:
		return uint64(b)
	case b == nBytesCode:
		switch n := d.readByte(); n {
		case 4:
			return uint64(d.readUint32())
		case 8:
			return d.readUint64()
		default:
			d.failf("DecodeUint: bad length %d", n)
		}
	default:
		d.badcode(b)
	}
	return 0
}

// EncodeInt encodes a signed integer.
func (e *Encoder) EncodeInt(i int64) {
	// Encode small negative as well as small positive integers efficiently.
	// Algorithm from gob; see "Encoding Details" at https://pkg.go.dev/encoding/gob.
	var u uint64
	if i < 0 {
		u = (^uint64(i) << 1) | 1 // complement i, bit 0 is 1
	} else {
		u = (uint64(i) << 1) // do not complement i, bit 0 is 0
	}
	e.EncodeUint(u)
}

// DecodeInt decodes a signed integer.
func (d *Decoder) DecodeInt() int64 {
	u := d.DecodeUint()
	if u&1 == 1 {
		return int64(^(u >> 1))
	}
	return int64(u >> 1)
}

// encodeLen encodes the length of a byte sequence.
func (e *Encoder) encodeLen(n int) {
	e.writeByte(nBytesCode)
	e.EncodeUint(uint64(n))
}

// decodeLen decodes the length of a byte sequence.
func (d *Decoder) decodeLen() int {
	if b := d.readByte(); b != nBytesCode {
		d.badcode(b)
	}
	return int(d.DecodeUint())
}

// EncodeBytes encodes a byte slice.
func (e *Encoder) EncodeBytes(b []byte) {
	e.encodeLen(len(b))
	e.writeBytes(b)
}

// DecodeBytes decodes a byte slice.
// It does no copying.
func (d *Decoder) DecodeBytes() []byte {
	return d.readBytes(d.decodeLen())
}

// EncodeString encodes a string.
func (e *Encoder) EncodeString(s string) {
	e.encodeLen(len(s))
	e.writeString(s)
}

// DecodeString decodes a string.
func (d *Decoder) DecodeString() string {
	return d.readString(d.decodeLen())
}

// EncodeBool encodes a bool.
func (e *Encoder) EncodeBool(b bool) {
	if b {
		e.writeByte(1)
	} else {
		e.writeByte(0)
	}
}

// DecodeBool decodes a bool.
func (d *Decoder) DecodeBool() bool {
	b := d.readByte()
	switch b {
	case 0:
		return false
	case 1:
		return true
	default:
		d.failf("bad bool: %d", b)
		return false
	}
}

// EncodeFloat encodes a float64.
func (e *Encoder) EncodeFloat(f float64) {
	e.EncodeUint(math.Float64bits(f))
}

// DecodeFloat decodes a float64.
func (d *Decoder) DecodeFloat() float64 {
	return math.Float64frombits(d.DecodeUint())
}

// StartList should be called before encoding any sequence of variable-length
// values.
func (e *Encoder) StartList(len int) {
	e.writeByte(nValuesCode)
	e.EncodeUint(uint64(len))
}

// StartList should be called before decoding any sequence of variable-length
// values. It returns -1 if the encoded list was nil. Otherwise, it returns the
// length of the sequence.
func (d *Decoder) StartList() int {
	b := d.readByte()
	if b == 0 { // used for nil
		return -1
	}
	if b != nValuesCode {
		d.badcode(b)
		return 0
	}
	return int(d.DecodeUint())
}
