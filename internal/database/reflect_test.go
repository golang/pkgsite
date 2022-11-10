// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql/driver"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lib/pq"
)

type testStruct struct {
	Name  string
	Score int
	Slice []int64
	Ptr   *int64
	Bytes []byte
}

func TestNullPtr(t *testing.T) {
	var ip *int64
	np := NullPtr(&ip)
	if err := np.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := np.Value(); got != driver.Value(nil) || err != nil {
		t.Errorf("got (%v, %v), want (nil, nil)", got, err)
	}
	if ip != nil {
		t.Error("ts.Ptr is not nil")
	}

	const want int64 = 3
	if err := np.Scan(want); err != nil {
		t.Fatal(err)
	}
	if got, err := np.Value(); got != want || err != nil {
		t.Errorf("got (%v, %v), want (%d, nil)", got, err, want)
	}
	if got := *ip; got != want {
		t.Errorf("*ip = %d, want %d", got, want)
	}
}

func TestStructScanner(t *testing.T) {
	f := StructScanner[testStruct]()
	var s testStruct
	args := f(&s)
	*args[0].(*string) = "foo"
	*args[1].(*int) = 3
	*args[2].(*pq.Int64Array) = []int64{1, 2, 3}
	if err := args[3].(nullPtr).Scan(int64(9)); err != nil {
		t.Fatal(err)
	}
	*args[4].(*[]byte) = []byte("abc")
	want := testStruct{"foo", 3, []int64{1, 2, 3}, intptr(9), []byte("abc")}
	if !cmp.Equal(s, want) {
		t.Errorf("got %+v, want %+v", s, want)
	}
}

func TestCollectStructs(t *testing.T) {
	ctx := context.Background()
	if _, err := testDB.Exec(ctx, "DROP TABLE IF EXISTS structs"); err != nil {
		t.Fatal(err)
	}
	_, err := testDB.Exec(ctx, `
		CREATE TABLE structs (
			name     text NOT NULL,
			score    integer NOT NULL,
			slice    integer[],
			nullable integer,
			bytes    bytea
		)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.BulkInsert(ctx, "structs", []string{"name", "score", "slice", "nullable", "bytes"}, []any{
		"A", 1, pq.Array([]int64(nil)), 7, nil,
		"B", 2, pq.Array([]int64{1, 2}), -8, []byte("abc"),
		"C", 3, pq.Array([]int64{}), nil, []byte("def"),
	}, ""); err != nil {
		t.Fatal(err)
	}

	query := `SELECT name, score, slice, nullable, bytes FROM structs`
	got, err := CollectStructs[testStruct](ctx, testDB, query)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []testStruct{
		{"A", 1, nil, intptr(7), nil},
		{"B", 2, []int64{1, 2}, intptr(-8), []byte("abc")},
		{"C", 3, []int64{}, nil, []byte("def")},
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// Same, but with a slice of struct pointers.
	gotp, err := CollectStructPtrs[testStruct](ctx, testDB, query)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(gotp, func(i, j int) bool { return got[i].Name < got[j].Name })
	var wantp []*testStruct
	for _, w := range want {
		ts := w
		wantp = append(wantp, &ts)
	}
	if !cmp.Equal(gotp, wantp) {
		t.Errorf("got %+v, want %+v", gotp, wantp)
	}
}

func intptr(i int64) *int64 {
	return &i
}
