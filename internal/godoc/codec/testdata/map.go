// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// DO NOT MODIFY. Generated code.

package somepkg

import (
	"golang.org/x/pkgsite/internal/godoc/codec"
)

func encode_map_string_bool(e *codec.Encoder, m map[string]bool) {
	if m == nil {
		e.EncodeUint(0)
		return
	}
	e.StartList(2 * len(m))
	for k, v := range m {
		e.EncodeString(k)
		e.EncodeBool(v)
	}
}

func decode_map_string_bool(d *codec.Decoder, p *map[string]bool) {
	n2 := d.StartList()
	if n2 < 0 {
		return
	}
	n := n2 / 2
	m := make(map[string]bool, n)
	var k string
	var v bool
	for i := 0; i < n; i++ {
		k = d.DecodeString()
		v = d.DecodeBool()
		m[k] = v
	}
	*p = m
}

func init() {
	codec.Register(map[string]bool(nil),
		func(e *codec.Encoder, x interface{}) { encode_map_string_bool(e, x.(map[string]bool)) },
		func(d *codec.Decoder) interface{} { var x map[string]bool; decode_map_string_bool(d, &x); return x })
}
