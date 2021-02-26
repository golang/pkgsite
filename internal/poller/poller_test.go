// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package poller

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

type numError struct {
	num int
}

func (e numError) Error() string { return strconv.Itoa(e.num) }

func Test(t *testing.T) {
	var (
		mu          sync.Mutex
		goods, bads []int
	)

	cur := -1
	getter := func(context.Context) (interface{}, error) {
		// Even: success; odd: failure.
		cur++
		if cur%2 == 0 {
			return cur, nil
		}
		return nil, numError{cur}
	}

	onError := func(err error) {
		mu.Lock()
		bads = append(bads, err.(numError).num)
		mu.Unlock()
	}

	p := New(cur, getter, onError)
	if got, want := p.Current(), cur; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx, 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // wait for first poll
	for i := 0; i < 10; i++ {
		goods = append(goods, p.Current().(int))
		time.Sleep(60 * time.Millisecond)
	}
	cancel()
	// Expect goods to be all even and non-decreasing.
	for i, g := range goods {
		if g%2 != 0 || (i > 0 && goods[i-1] > g) {
			t.Errorf("incorrect 'good' value %d", g)
		}
	}
	// Expect bads to be consecutive odd numbers.
	mu.Lock()
	bs := bads
	mu.Unlock()
	for i, b := range bs {
		if b%2 == 0 || (i > 0 && bads[i-1]+2 != b) {
			t.Errorf("incorrect 'bad' value %d", b)
		}
	}
}
