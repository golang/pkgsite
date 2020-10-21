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
