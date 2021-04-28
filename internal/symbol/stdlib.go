// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

var pathToExceptions = map[string]map[string]bool{
	// RawMessage.MarshalJSON was introduced in go1, but in go1.8 the receiver
	// was changed from a point (*RawMessage) to a value (RawMessage).
	// See https://golang.org/doc/go1.8#encoding_json. This resulted in
	// golang.org/pkg/encoding/json to show that this symbol was introduced in
	// go1.8. We want to show go1.1 on pkg.go.dev.
	"encoding/json": {
		"RawMessage.MarshalJSON": true,
	},
	// Package syscall doesn't follow the Go 1 compatibility promise like all
	// other normal packages in the standard library. See
	// https://golang.org/s/go1.4-syscall and its API varies significantly
	// between different GOOS/GOARCH pairs.
	"syscall": {
		// RawSockaddrInet6 is listed in api/go1.txt as:
		//     pkg syscall (darwin-386), type RawSockaddrInet6 struct
		// It is listed in api/go1.1.txt as:
		//     pkg syscall type RawSockaddrInet6 struct
		// As a result, golang.org/pkg/syscall only recognizes its existence as
		// of go1.1.
		// The same is true for RawSockaddrUnix, TimespecToNsec, and
		// NsecToTimespec.
		"RawSockaddrInet6":          true,
		"RawSockaddrInet6.Addr":     true,
		"RawSockaddrInet6.Flowinfo": true,
		"RawSockaddrInet6.Port":     true,
		"RawSockaddrInet6.Scope_id": true,
		"RawSockaddrUnix":           true,
		"TimespecToNsec":            true,
		"NsecToTimespec":            true,
	},
}

// pathToEmbeddedMethods contains methods that appear on a struct, because it
// was added to an embedded struct. pkgsite does not currently support embedding,
// so these methods are skipped by when comparing the stdlib API for now.
var pathToEmbeddedMethods = map[string]map[string]string{
	"archive/zip": {
		// https://pkg.go.dev/archive/zip@go1.6#ReadCloser
		// Embedded https://pkg.go.dev/archive/zip#Reader.RegisterDecompressor
		"ReadCloser.RegisterDecompressor": "v1.6.0",
		// https://pkg.go.dev/archive/zip@go1.16#ReadCloser
		// method Open was added to embedded struct in go1.16
		// https://pkg.go.dev/archive/zip@go1.16#Reader.Open
		// Release notes: https://golang.org/doc/go1.16#archive/zip
		"ReadCloser.Open": "v1.16.0",
	},
	"bufio": {
		// https://pkg.go.dev/bufio@go1.1#ReadWriter
		// Embedded https://pkg.go.dev/bufio#Writer.ReadFrom
		"ReadWriter.ReadFrom": "v1.1.0",
		// https://pkg.go.dev/bufio@go1.1#ReadWriter
		// Embedded https://pkg.go.dev/bufio#Reader.WriteTo
		"ReadWriter.WriteTo": "v1.1.0",
		// https://pkg.go.dev/bufio@go1.5#ReadWriter
		// Embedded https://pkg.go.dev/bufio#Reader.Discard
		"ReadWriter.Discard": "v1.5.0",
	},
	"crypto/rsa": {
		// https://pkg.go.dev/crypto/rsa@go1.11#PrivateKey
		// Embedded https://pkg.go.dev/crypto/rsa#PublicKey.Size
		"PrivateKey.Size": "v1.11.0",
	},
	"debug/dwarf": {
		// https://pkg.go.dev/debug/dwarf@go1.13#UnsupportedType
		// Embedded https://pkg.go.dev/debug/dwarf#CommonType.Common
		"UnsupportedType.Common": "v1.13.0",
		// https://pkg.go.dev/debug/dwarf@go1.13#UnsupportedType
		// Embedded https://pkg.go.dev/debug/dwarf#CommonType.Size
		"UnsupportedType.Size": "v1.13.0",
		// https://pkg.go.dev/debug/dwarf@go1.4#UnspecifiedType
		// Embedded https://pkg.go.dev/debug/dwarf#CommonType.Common
		"UnspecifiedType.Common": "v1.4.0",
		// https://pkg.go.dev/debug/dwarf@go1.4#UnspecifiedType
		// Embedded https://pkg.go.dev/debug/dwarf#CommonType.Size
		"UnspecifiedType.Size": "v1.4.0",
		// https://pkg.go.dev/debug/dwarf@go1.4#UnspecifiedType
		// Embedded https://pkg.go.dev/debug/dwarf#BasicType.Basic
		"UnspecifiedType.Basic": "v1.4.0",
		// https://pkg.go.dev/debug/dwarf@go1.4#UnspecifiedType
		// Embedded https://pkg.go.dev/debug/dwarf#BasicType.String
		"UnspecifiedType.String": "v1.4.0",
	},
	"debug/macho": {
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.Close
		"FatArch.Close": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.DWARF
		"FatArch.DWARF": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.ImportedLibraries
		"FatArch.ImportedLibraries": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.ImportedSymbols
		"FatArch.ImportedSymbols": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.Section
		"FatArch.Section": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.3#FatArch
		// Embedded https://pkg.go.dev/debug/macho#File.Segment
		"FatArch.Segment": "v1.3.0",
		// https://pkg.go.dev/debug/macho@go1.10#Rpath
		// Embeddded https://pkg.go.dev/debug/macho#LoadBytes.Raw
		"Rpath.Raw": "v1.10.0",
	},
	"debug/plan9obj": {
		// https://pkg.go.dev/debug/plan9obj@go1.3#Section
		// Embedded https://pkg.go.dev/io#ReaderAt.ReadAt
		"Section.ReadAt": "v1.3.0",
	},
	"go/types": {
		// https://pkg.go.dev/go/types@go1.5#Checker
		// Embedded https://pkg.go.dev/go/types#Info.ObjectOf
		"Checker.ObjectOf": "v1.5.0",
		// https://pkg.go.dev/go/types@go1.5#Checker
		// Embedded https://pkg.go.dev/go/types@go1.5#Info.TypeOf
		"Checker.TypeOf": "v1.5.0",
	},
	"image": {
		// https://pkg.go.dev/image@go1.6#NYCbCrA
		// Embedded https://pkg.go.dev/image#YCbCr.Bounds
		"NYCbCrA.Bounds": "v1.6.0",
		// https://pkg.go.dev/image@go1.6#NYCbCrA
		// Embedded https://pkg.go.dev/image@go1.6#YCbCr.COffset
		"NYCbCrA.COffset": "v1.6.0",
		// https://pkg.go.dev/image@go1.6#NYCbCrA
		// Embedded https://pkg.go.dev/image@go1.6#YCbCr.YCbCrAt
		"NYCbCrA.YCbCrAt": "v1.6.0",
		// https://pkg.go.dev/image@go1.6#NYCbCrA
		// Embedded https://pkg.go.dev/image@go1.6#YCbCr.YOffset
		"NYCbCrA.YOffset": "v1.6.0",
	},
	"os/exec": {
		// https://pkg.go.dev/os/exec@go1.12#ExitError
		// Embedded https://pkg.go.dev/os#ProcessState.ExitCode
		"ExitError.ExitCode": "v1.12.0",
	},
	"runtime": {
		// https://pkg.go.dev/runtime@go1.1#BlockProfileRecord
		// Embedded https://pkg.go.dev/runtime#StackRecord.Stack
		"BlockProfileRecord.Stack": "v1.1.0",
	},
	"text/template": {
		// https://pkg.go.dev/text/template@go1.2#Template
		// Embedded https://pkg.go.dev/text/template/parse#Tree.ErrorContext
		"Template.ErrorContext": "v1.1.0",
		// https://pkg.go.dev/text/template@go1.2#Template
		// Embedded https://pkg.go.dev/text/template/parse#Tree.Copy
		"Template.Copy": "v1.2.0",
	},
	"text/template/parse": {
		// https://pkg.go.dev/text/template/parse@go1.1#ActionNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"ActionNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#BoolNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"BoolNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#Branch
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"BranchNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#ChainNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"ChainNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#ChainNode
		// Embedded https://pkg.go.dev/text/template/parse@go1.1#NodeType.Type
		"ChainNode.Type": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#CommandNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"CommandNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.16#CommentNode
		// Embedded https://pkg.go.dev/text/template/parse@go1.16#Pos.Position
		// Release notes: https://golang.org/doc/go1.16#text/template/parse
		"CommentNode.Position": "v1.16.0",
		// https://pkg.go.dev/text/template/parse@go1.16#CommentNode
		// Embedded https://pkg.go.dev/text/template/parse@go1.16#NodeType.Type
		// Release notes: https://golang.org/doc/go1.16#text/template/parse
		"CommentNode.Type": "v1.16.0",
		// https://pkg.go.dev/text/template/parse@go1.1#DotNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"DotNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#FieldNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"FieldNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#IdentifierNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"IdentifierNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#IfNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"IfNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#ListNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"ListNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#NilNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"NilNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#NumberNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"NumberNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#PipeNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"PipeNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#RangeNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"RangeNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#StringNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"StringNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#TemplateNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"TemplateNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#TextNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"TextNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#VariableNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"VariableNode.Position": "v1.1.0",
		// https://pkg.go.dev/text/template/parse@go1.1#WithNode
		// Embedded https://pkg.go.dev/text/template/parse#Pos.Position
		"WithNode.Position": "v1.1.0",
	},
}
