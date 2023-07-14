// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/lib/pq"
)

// StructScanner returns a function that, when called on a
// struct pointer of its argument type, returns a slice of arguments suitable for
// Row.Scan or Rows.Scan. The call to either Scan will populate the exported
// fields of the struct in the order they appear in the type definition.
//
// StructScanner panics if p is not a struct or a pointer to a struct.
// The function it returns will panic if its argument is not a pointer
// to a struct.
//
// Example:
//
//	type Player struct { Name string; Score int }
//	playerScanArgs := database.StructScanner(Player{})
//	err := db.RunQuery(ctx, "SELECT name, score FROM players", func(rows *sql.Rows) error {
//	    var p Player
//	    if err := rows.Scan(playerScanArgs(&p)...); err != nil {
//	        return err
//	    }
//	    // use p
//	    return nil
//	})
func StructScanner[T any]() func(p *T) []any {
	return structScannerForType[T]()
}

type fieldInfo struct {
	num  int // to pass to v.Field
	kind reflect.Kind
}

func structScannerForType[T any]() func(p *T) []any {
	var x T
	t := reflect.TypeOf(x)
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("%s is not a struct", t))
	}

	// Collect the numbers of the exported fields.
	var fieldInfos []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		r, _ := utf8.DecodeRuneInString(t.Field(i).Name)
		if unicode.IsUpper(r) {
			fieldInfos = append(fieldInfos, fieldInfo{i, t.Field(i).Type.Kind()})
		}
	}
	// Return a function that gets pointers to the exported fields.
	return func(p *T) []any {
		v := reflect.ValueOf(p).Elem()
		var ps []any
		for _, info := range fieldInfos {
			p := v.Field(info.num).Addr().Interface()
			switch info.kind {
			case reflect.Slice:
				if _, ok := p.(*[]byte); !ok {
					p = pq.Array(p)
				}
			case reflect.Ptr:
				p = NullPtr(p)
			default:
			}
			ps = append(ps, p)
		}
		return ps
	}
}

// NullPtr is for scanning nullable database columns into pointer variables or
// fields. When given a pointer to a pointer to some type T, it returns a
// value that can be passed to a Scan function. If the corresponding column is
// nil, the variable will be set to nil. Otherwise, it will be set to a newly
// allocated pointer to the column value.
func NullPtr(p any) nullPtr {
	v := reflect.ValueOf(p)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Ptr {
		panic("NullPtr arg must be pointer to pointer")
	}
	return nullPtr{v}
}

type nullPtr struct {
	// ptr is a pointer to a pointer to something: **T
	ptr reflect.Value
}

func (n nullPtr) Scan(value any) error {
	// n.ptr is like a variable v of type **T
	ntype := n.ptr.Elem().Type() // T
	if value == nil {
		n.ptr.Elem().Set(reflect.Zero(ntype)) // *v = nil
	} else {
		p := reflect.New(ntype.Elem())       // p := new(T)
		p.Elem().Set(reflect.ValueOf(value)) // *p = value
		n.ptr.Elem().Set(p)                  // *v = p
	}
	return nil
}

func (n nullPtr) Value() (driver.Value, error) {
	if n.ptr.Elem().IsNil() {
		return nil, nil
	}
	return n.ptr.Elem().Elem().Interface(), nil
}

// CollectStructs scans the rows from the query into structs and returns a slice of them.
// Example:
//
//	type Player struct { Name string; Score int }
//	var players []Player
//	err := db.CollectStructs(ctx, &players, "SELECT name, score FROM players")
func CollectStructs[T any](ctx context.Context, db *DB, query string, args ...any) ([]T, error) {
	scanner := structScannerForType[T]()
	var ts []T
	err := db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var s T
		if err := rows.Scan(scanner(&s)...); err != nil {
			return err
		}
		ts = append(ts, s)
		return nil
	}, args...)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

func CollectStructPtrs[T any](ctx context.Context, db *DB, query string, args ...any) ([]*T, error) {
	scanner := structScannerForType[T]()
	var ts []*T
	err := db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var s T
		if err := rows.Scan(scanner(&s)...); err != nil {
			return err
		}
		ts = append(ts, &s)
		return nil
	}, args...)
	if err != nil {
		return nil, err
	}
	return ts, nil
}
