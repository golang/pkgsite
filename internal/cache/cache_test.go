// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/alicebob/miniredis/v2 pulls in
// github.com/yuin/gopher-lua which uses a non
// build-tag-guarded use of the syscall package.
//go:build !plan9

package cache

import (
	"context"
	"sort"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-cmp/cmp"
)

func TestBasics(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := New(redis.NewClient(&redis.Options{Addr: s.Addr()}))

	val := []byte("value")
	must(t, c.Put(ctx, "key", val, 0))
	got, err := c.Get(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(got, val) {
		t.Fatalf("got %v, want %v", got, val)
	}

	must(t, c.Delete(ctx, "key"))
	got, err = c.Get(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestDeletePrefix(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := New(redis.NewClient(&redis.Options{Addr: s.Addr()}))

	check := func(want []string) {
		t.Helper()
		got, err := c.client.Keys(ctx, "*").Result()
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(want)
		sort.Strings(got)
		if !cmp.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	}

	all := []string{"a", "b", "c", "a@x", "a/x"}
	for _, k := range all {
		must(t, c.Put(ctx, k, []byte("value"), 0))
	}
	check(all)

	scanCount = 1
	must(t, c.DeletePrefix(ctx, "a"))
	check([]string{"b", "c"})

	must(t, c.Clear(ctx))
	check([]string{})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
