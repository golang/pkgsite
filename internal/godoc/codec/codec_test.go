// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"math"
	"strings"
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
	d.decodeInitial()
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

func TestUint(t *testing.T) {
	e := NewEncoder()
	uints := []uint64{99, 999, math.MaxUint32 + 1}
	for _, u := range uints {
		e.EncodeUint(u)
	}
	d := NewDecoder(e.Bytes())
	d.decodeInitial()
	for _, want := range uints {
		if got := d.DecodeUint(); got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	}
}

func TestInt(t *testing.T) {
	e := NewEncoder()
	ints := []int64{99, 999, math.MaxUint32 + 1, -123}
	for _, i := range ints {
		e.EncodeInt(i)
	}
	d := NewDecoder(e.Bytes())
	d.decodeInitial()
	for _, want := range ints {
		if got := d.DecodeInt(); got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	}
}

func TestBasicTypes(t *testing.T) {
	e := NewEncoder()
	var (
		by = []byte{1, 2, 3}
		s  = "hello"
		b  = true
		f  = 3.14
	)
	e.EncodeBytes(by)
	e.EncodeString(s)
	e.EncodeBool(b)
	e.EncodeFloat(f)

	d := NewDecoder(e.Bytes())
	d.decodeInitial()
	gots := []any{
		d.DecodeBytes(),
		d.DecodeString(),
		d.DecodeBool(),
		d.DecodeFloat(),
	}
	wants := []any{by, s, b, f}
	if !cmp.Equal(gots, wants) {
		t.Errorf("got %v, want %v", gots, wants)
	}
}

func TestList(t *testing.T) {
	e := NewEncoder()
	want := []string{"Green", "eggs", "and", "ham"}
	e.StartList(len(want))
	for _, s := range want {
		e.EncodeString(s)
	}

	d := NewDecoder(e.Bytes())
	d.decodeInitial()
	n := d.StartList()
	if n < 0 {
		t.Fatal("got nil")
	}
	got := make([]string, n)
	for i := 0; i < n; i++ {
		got[i] = d.DecodeString()
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAny(t *testing.T) {
	want := []any{"bar", nil, 1, -5, 98.6, uint64(1 << 63), "Luke Luck likes lakes", true}
	e := NewEncoder()
	for _, w := range want {
		e.EncodeAny(w)
	}

	d := NewDecoder(e.Bytes())
	d.decodeInitial()
	for _, w := range want {
		g := d.DecodeAny()
		if g != w {
			t.Errorf("got %v, want %v", g, w)
		}
	}
}

func TestEncodeDecode(t *testing.T) {
	want := []any{"bar", nil, 1, -5, 98.6, uint64(1 << 63), "Luke Luck likes lakes", true}
	e := NewEncoder()
	for _, w := range want {
		if err := e.Encode(w); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDecoder(e.Bytes())
	for _, w := range want {
		g, err := d.Decode()
		if err != nil {
			t.Fatal(err)
		}
		if g != w {
			t.Errorf("got %v, want %v", g, w)
		}
	}
}

func TestEncodeErrors(t *testing.T) {
	// The only encoding error is an unregistered type.
	e := NewEncoder()
	type MyInt int
	checkMessage(t, e.Encode(MyInt(0)), "unregistered")
}

func TestDecodeErrors(t *testing.T) {
	for _, test := range []struct {
		offset  int
		change  byte
		message string
	}{
		// d.buf[d.i:] should look like: nValues 2 0 nBytes 4 ...
		// Induce errors by changing some bytes.
		{0, startCode, "bad code"},   // mess with the initial code
		{1, 5, "bad list length"},    // mess with the list length
		{2, 1, "out of range"},       // mess with the type number
		{3, nValuesCode, "bad code"}, // mess with the uint code
		{4, 5, "bad length"},         // mess with the uint length
	} {
		d := NewDecoder(mustEncode(t, uint64(3000)))
		d.decodeInitial()
		d.buf[d.i+test.offset] = test.change
		_, err := d.Decode()
		checkMessage(t, err, test.message)
	}
}

func mustEncode(t *testing.T, x any) []byte {
	t.Helper()
	e := NewEncoder()
	if err := e.Encode(x); err != nil {
		t.Fatal(err)
	}
	return e.Bytes()
}

func checkMessage(t *testing.T, err error, target string) {
	t.Helper()
	if err == nil {
		t.Error("want error, got nil")
	}
	if !strings.Contains(err.Error(), target) {
		t.Errorf("error %q does not contain %q", err, target)
	}
}

func TestSkip(t *testing.T) {
	e := NewEncoder()
	values := []any{
		1,
		false,
		"yes",
		"no",
		65000,
	}
	for _, v := range values {
		if err := e.Encode(v); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDecoder(e.Bytes())
	// Skip odd indexes.
	for i, want := range values {
		if i%2 == 0 {
			got, err := d.Decode()
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		} else {
			d.skip()
		}
	}
}

type node struct {
	Value int
	Next  *node
}

func TestSharing(t *testing.T) {
	n := &node{Value: 1, Next: &node{Value: 2}}
	n.Next.Next = n // create a cycle
	d := NewDecoder(mustEncode(t, n))
	g, err := d.Decode()
	if err != nil {
		t.Fatal(err)
	}
	got := g.(*node)
	if !cmp.Equal(got, n) {
		t.Error("did not preserve cycle")
	}
}

// The following three functions were generated with
//
//	Generate(os.Stdout, "p", node{})
//
// and pasted here. They should be kept in sync
// with the output of Generate.
func encode_node(e *Encoder, x *node) {
	if !e.StartStruct(x == nil, x) {
		return
	}
	if x.Value != 0 {
		e.EncodeUint(0)
		e.EncodeInt(int64(x.Value))
	}
	if x.Next != nil {
		e.EncodeUint(1)
		encode_node(e, x.Next)
	}
	e.EndStruct()
}

func decode_node(d *Decoder, p **node) {
	proceed, ref := d.StartStruct()
	if !proceed {
		return
	}
	if ref != nil {
		*p = ref.(*node)
		return
	}
	var x node
	d.StoreRef(&x)
	for {
		n := d.NextStructField()
		if n < 0 {
			break
		}
		switch n {
		case 0:
			x.Value = int(d.DecodeInt())
		case 1:
			decode_node(d, &x.Next)

		default:
			d.UnknownField("node", n)
		}
		*p = &x
	}
}

func init() {
	Register(&node{},
		func(e *Encoder, x any) { encode_node(e, x.(*node)) },
		func(d *Decoder) any {
			var x *node
			decode_node(d, &x)
			return x
		})
}
