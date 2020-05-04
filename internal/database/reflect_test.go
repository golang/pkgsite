// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lib/pq"
)

type testStruct struct {
	Name  string
	Score int
	Slice []int64
}

func TestStructScanner(t *testing.T) {
	var s testStruct
	f := StructScanner(s)
	args := f(&s)
	*args[0].(*string) = "foo"
	*args[1].(*int) = 3
	*args[2].(*pq.Int64Array) = []int64{1, 2, 3}
	want := testStruct{"foo", 3, []int64{1, 2, 3}}
	if !cmp.Equal(s, want) {
		t.Errorf("got %+v, want %+v", s, want)
	}

	// StructScanner should also work on pointers to structs.
	_ = StructScanner(&s)
}

func TestCollectStructs(t *testing.T) {
	ctx := context.Background()
	if _, err := testDB.Exec(ctx, "DROP TABLE IF EXISTS structs"); err != nil {
		t.Fatal(err)
	}
	_, err := testDB.Exec(ctx, `
		CREATE TABLE structs (
			name text NOT NULL,
			score integer NOT NULL,
			slice integer[]
		)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.BulkInsert(ctx, "structs", []string{"name", "score", "slice"}, []interface{}{
		"A", 1, pq.Array([]int64(nil)),
		"B", 2, pq.Array([]int64{1, 2}),
		"C", 3, pq.Array([]int64{}),
	}, ""); err != nil {
		t.Fatal(err)
	}

	query := `SELECT name, score, slice FROM structs`
	var got []testStruct
	if err := testDB.CollectStructs(ctx, &got, query); err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []testStruct{{"A", 1, nil}, {"B", 2, []int64{1, 2}}, {"C", 3, []int64{}}}
	if !cmp.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// Same, but with a slice of struct pointers.
	var gotp []*testStruct
	if err := testDB.CollectStructs(ctx, &gotp, query); err != nil {
		t.Fatal(err)
	}
	sort.Slice(gotp, func(i, j int) bool { return got[i].Name < got[j].Name })
	wantp := []*testStruct{{"A", 1, nil}, {"B", 2, []int64{1, 2}}, {"C", 3, []int64{}}}
	if !cmp.Equal(gotp, wantp) {
		t.Errorf("got %+v, want %+v", gotp, wantp)
	}

}
