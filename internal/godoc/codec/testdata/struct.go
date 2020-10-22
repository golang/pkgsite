// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// DO NOT MODIFY. Generated code.

package somepkg

import (
	"go/ast"

	"go/token"

	"golang.org/x/pkgsite/internal/godoc/codec"
)

func encode_ast_BasicLit(e *codec.Encoder, x *ast.BasicLit) {
	if !e.StartStruct(x == nil, x) {
		return
	}
	if x.ValuePos != 0 {
		e.EncodeUint(0)
		e.EncodeInt(int64(x.ValuePos))
	}
	if x.Kind != 0 {
		e.EncodeUint(1)
		e.EncodeInt(int64(x.Kind))
	}
	if x.Value != "" {
		e.EncodeUint(2)
		e.EncodeString(x.Value)
	}
	e.EndStruct()
}

func decode_ast_BasicLit(d *codec.Decoder, p **ast.BasicLit) {
	proceed, ref := d.StartStruct()
	if !proceed {
		return
	}
	if ref != nil {
		*p = ref.(*ast.BasicLit)
		return
	}
	var x ast.BasicLit
	d.StoreRef(&x)
	for {
		n := d.NextStructField()
		if n < 0 {
			break
		}
		switch n {
		case 0:
			x.ValuePos = token.Pos(d.DecodeInt())
		case 1:
			x.Kind = token.Token(d.DecodeInt())
		case 2:
			x.Value = d.DecodeString()

		default:
			d.UnknownField("ast.BasicLit", n)
		}
		*p = &x
	}
}

func init() {
	codec.Register(&ast.BasicLit{},
		func(e *codec.Encoder, x interface{}) { encode_ast_BasicLit(e, x.(*ast.BasicLit)) },
		func(d *codec.Decoder) interface{} {
			var x *ast.BasicLit
			decode_ast_BasicLit(d, &x)
			return x
		})
}
