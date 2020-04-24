// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"unicode"
	"unicode/utf8"
)

// StructScanner takes a struct and returns a function that, when called on a
// struct pointer of that type, returns a slice of arguments suitable for
// Row.Scan or Rows.Scan. The call to either Scan will populate the exported
// fields of the struct in the order they appear in the type definition.
//
// StructScanner panics if p is not a struct or a pointer to a struct.
// The function it returns will panic if its argument is not a pointer
// to a struct.
//
// Example:
//   type Player struct { Name string; Score int }
//   playerScanArgs := database.StructScanner(Player{})
//   err := db.RunQuery(ctx, "SELECT name, score FROM players", func(rows *sql.Rows) error {
//       var p Player
//       if err := rows.Scan(playerScanArgs(&p)...); err != nil {
//           return err
//       }
//       // use p
//       return nil
//   })
func StructScanner(s interface{}) func(p interface{}) []interface{} {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return structScannerForType(v.Type())
}

func structScannerForType(t reflect.Type) func(p interface{}) []interface{} {
	// Collect the numbers of the exported fields.
	var fieldNums []int
	for i := 0; i < t.NumField(); i++ {
		r, _ := utf8.DecodeRuneInString(t.Field(i).Name)
		if unicode.IsUpper(r) {
			fieldNums = append(fieldNums, i)
		}
	}
	// Return a function that gets pointers to the exported fields.
	return func(p interface{}) []interface{} {
		v := reflect.ValueOf(p).Elem()
		var ps []interface{}
		for _, i := range fieldNums {
			ps = append(ps, v.Field(i).Addr().Interface())
		}
		return ps
	}
}

// CollectStructs scans the the rows from the query into structs and appends
// them to pslice, which must be a pointer to a slice of structs.
// Example:
//   type Player struct { Name string; Score int }
//   var players []Player
//   err := db.CollectStructs(ctx, "SELECT name, score FROM players", &players)
func (db *DB) CollectStructs(ctx context.Context, query string, pslice interface{}, args ...interface{}) error {
	v := reflect.ValueOf(pslice)
	if v.Kind() != reflect.Ptr {
		return errors.New("collectStructs: arg is not a pointer")
	}
	ve := v.Elem()
	if ve.Kind() != reflect.Slice {
		return errors.New("collectStructs: arg is not a pointer to a slice")
	}
	et := ve.Type().Elem() // slice element type
	scanner := structScannerForType(et)
	err := db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		e := reflect.New(et)
		if err := rows.Scan(scanner(e.Interface())...); err != nil {
			return err
		}
		ve = reflect.Append(ve, e.Elem())
		return nil
	}, args...)
	if err != nil {
		return err
	}
	v.Elem().Set(ve)
	return nil
}
