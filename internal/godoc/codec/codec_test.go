// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLowLevelIO(t *testing.T) {
	var (
		b   byte   = 15
		bs  []byte = []byte{4, 10, 8}
		s          = "hello"
		u32 uint32 = 999
		u64 uint64 = math.MaxUint32 + 1
	)

	e := NewEncoder()
	e.writeByte(b)
	e.writeBytes(bs)
	e.writeString(s)
	e.writeUint32(u32)
	e.writeUint64(u64)

	d := NewDecoder(e.Bytes())
	if got := d.readByte(); got != b {
		t.Fatalf("got %d, want %d", got, b)
	}
	if got := d.readBytes(len(bs)); !cmp.Equal(got, bs) {
		t.Fatalf("got %v, want %v", got, bs)
	}
	if got := d.readString(len(s)); got != s {
		t.Fatalf("got %q, want %q", got, s)
	}
	if got := d.readUint32(); got != u32 {
		t.Errorf("got %d, want %d", got, u32)
	}
	if got := d.readUint64(); got != u64 {
		t.Errorf("got %d, want %d", got, u64)
	}
}
