// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"go/token"
	"io"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/codec"
)

// The encoding type identifies the encoding being used, to distinguish them
// when reading from the DB.
const (
	encodingTypeLen  = 4 // all encoding types must be this many bytes
	fastEncodingType = "AST2"
)

// ErrInvalidEncodingType is returned when the data to DecodePackage has an
// invalid encoding type.
var ErrInvalidEncodingType = fmt.Errorf("want initial bytes to be %q but they aren't", fastEncodingType)

// Encode encodes a Package into a byte slice.
// During its operation, Encode modifies the AST,
// but it restores it to a state suitable for
// rendering before it returns.
func (p *Package) Encode(ctx context.Context) (_ []byte, err error) {
	defer derrors.Wrap(&err, "godoc.Package.Encode()")
	return p.fastEncode()
}

// DecodPackage decodes a byte slice encoded with Package.Encode into a Package.
func DecodePackage(data []byte) (_ *Package, err error) {
	defer derrors.Wrap(&err, "DecodePackage()")

	if len(data) < encodingTypeLen {
		return nil, ErrInvalidEncodingType
	}
	switch string(data[:encodingTypeLen]) {
	case fastEncodingType:
		return fastDecodePackage(data[encodingTypeLen:])
	default:
		return nil, ErrInvalidEncodingType
	}
}

func (p *Package) fastEncode() (_ []byte, err error) {
	defer derrors.Wrap(&err, "godoc.Package.FastEncode()")

	var buf bytes.Buffer
	io.WriteString(&buf, fastEncodingType)
	enc := codec.NewEncoder()
	fsb, err := fsetToBytes(p.Fset)
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(fsb); err != nil {
		return nil, err
	}
	if err := enc.Encode(&p.encPackage); err != nil {
		return nil, err
	}
	buf.Write(enc.Bytes())
	return buf.Bytes(), nil
}

func fastDecodePackage(data []byte) (_ *Package, err error) {
	defer derrors.Wrap(&err, "FastDecodePackage()")

	dec := codec.NewDecoder(data)
	x, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	fsetBytes, ok := x.([]byte)
	if !ok {
		return nil, fmt.Errorf("first decoded value is %T, wanted []byte", fsetBytes)
	}
	fset, err := fsetFromBytes(fsetBytes)
	if err != nil {
		return nil, err
	}
	x, err = dec.Decode()
	if err != nil {
		return nil, err
	}
	ep, ok := x.(*encPackage)
	if !ok {
		return nil, fmt.Errorf("second decoded value is %T, wanted *encPackage", ep)
	}
	return &Package{
		Fset:       fset,
		encPackage: *ep,
	}, nil
}

// token.FileSet uses some unexported types in its encoding, so we can't use our
// own codec from it. Instead we use gob and encode the resulting bytes.
func fsetToBytes(fset *token.FileSet) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := fset.Write(enc.Encode); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fsetFromBytes(data []byte) (*token.FileSet, error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	fset := token.NewFileSet()
	if err := fset.Read(dec.Decode); err != nil {
		return nil, err
	}
	return fset, nil
}

//go:generate go run gen_ast.go

// Used by the gen program to generate encodings for unexported types.
var TypesToGenerate = []any{&encPackage{}}
