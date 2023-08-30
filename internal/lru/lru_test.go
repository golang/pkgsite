// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lru

import "testing"

func TestSizeOne(t *testing.T) {
	c := New[int, int](1)
	c.Put(1, 1)
	got, gotOK := c.Get(1)
	if got != 1 && gotOK != true {
		t.Errorf("c.Get(1): got %v, %v, want %v, %v", got, gotOK, 1, true)
	}
	c.Put(2, 2)
	got, gotOK = c.Get(1)
	if got != 0 || gotOK != false {
		t.Errorf("c.Get(1): got %v, %v, want %v, %v", got, gotOK, 0, false)
	}
	got, gotOK = c.Get(2)
	if got != 2 && gotOK != true {
		t.Errorf("c.Get(2): got %v, %v, want %v, %v", got, gotOK, 2, true)
	}
}

func TestSizeFive(t *testing.T) {
	c := New[int, int](5)
	c.Put(1, 1)
	c.Put(2, 2)
	c.Put(3, 3)
	c.Put(4, 4)
	c.Put(5, 5)

	getHasKey := func(k int, has bool) {
		t.Helper()

		got, ok := c.Get(k)
		if has == false {
			if ok == true || got != 0 {
				t.Errorf("c.Get(%v): got %v, %v, want %v, %v", k, got, ok, 0, false)
			}
		} else if got != k || ok != true {
			t.Errorf("c.Get(%v): got %v, %v, want %v, %v", k, got, ok, k, true)
		}
	}

	getHasKey(3, true)
	getHasKey(2, true)
	getHasKey(1, true)
	getHasKey(5, true)
	getHasKey(4, true)
	c.Put(6, 6) // 3 gets evicted

	getHasKey(3, false)
	getHasKey(1, true)
	getHasKey(2, true)
	getHasKey(4, true)
	getHasKey(5, true)
	getHasKey(6, true)
	c.Put(7, 7)
	c.Put(8, 8) // 1 and 2 get evicted

	getHasKey(1, false)
	getHasKey(2, false)
	getHasKey(8, true)
	getHasKey(7, true)
	getHasKey(4, true)
	getHasKey(5, true)
	getHasKey(6, true)
	c.Put(9, 9)   // 8 gets evicted
	c.Put(10, 10) // 7 gets evicted
	c.Put(11, 11) // 4 gets evicted
	c.Put(12, 12) // 5 gets evicted
	c.Put(13, 13) // 6 gets evicted
	c.Put(14, 14) // 9 gets evicted

	getHasKey(4, false)
	getHasKey(5, false)
	getHasKey(6, false)
	getHasKey(7, false)
	getHasKey(8, false)
	getHasKey(9, false)
	getHasKey(10, true)
	getHasKey(11, true)
	getHasKey(12, true)
	getHasKey(13, true)
	getHasKey(14, true)
	c.Put(12, 12)

	getHasKey(10, true)
	getHasKey(11, true)
	getHasKey(12, true)
	getHasKey(13, true)
	getHasKey(14, true)
}
