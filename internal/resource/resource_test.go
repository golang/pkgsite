// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resource

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fake struct {
	id     int64
	closed bool
	mu     sync.Mutex
}

func (f *fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		panic("duplicate close")
	}
	f.closed = true
	return nil
}

func (f *fake) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// fakeTimer allows manual control over time-based events.
type fakeTimer struct {
	mu sync.Mutex
	fs []func()
}

func newFakeTimer() *fakeTimer {
	return &fakeTimer{}
}

func (t *fakeTimer) after(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fs = append(t.fs, f)
}

func (t *fakeTimer) advance(tt *testing.T) {
	tt.Helper()
	t.mu.Lock()
	fs := slices.Clone(t.fs)
	t.fs = nil
	t.mu.Unlock()
	if len(fs) == 0 {
		tt.Fatal("timer did not fire")
	}
	for _, f := range fs {
		f()
	}
	t.fs = nil
}

func TestResource_Reuse(t *testing.T) {
	var nextID atomic.Int64
	get := func() *fake {
		return &fake{id: nextID.Add(1)}
	}
	timer := newFakeTimer()
	r := newAfter(get, timer.after)

	f1, release1 := r.Get()
	if f1.id != 1 {
		t.Fatalf("f1.id = %d, want 1", f1.id)
	}

	f2, release2 := r.Get()
	if f2.id != 1 {
		t.Fatalf("f2.id = %d, want 1", f2.id)
	}

	release1()
	if f1.isClosed() {
		t.Fatal("f1 closed, want not closed")
	}
	release2()
	if f1.isClosed() {
		t.Fatal("f1 closed, want not closed")
	}

	// The resource holds its own reference, which is released by the timer.
	timer.advance(t)

	// Now all references are released, it should be closed.
	if !f1.isClosed() {
		t.Fatal("f1 not closed, want closed")
	}
}

func TestResource_Expire(t *testing.T) {
	var nextID atomic.Int64
	get := func() *fake {
		return &fake{id: nextID.Add(1)}
	}
	timer := newFakeTimer()
	r := newAfter(get, timer.after)

	f1, release1 := r.Get()
	if f1.id != 1 {
		t.Fatalf("f1.id = %d, want 1", f1.id)
	}
	release1() // Release our hold on it.

	// Advance time, causing the resource's internal reference to be released.
	timer.advance(t)

	if !f1.isClosed() {
		t.Fatal("f1 not closed, want closed")
	}

	f2, release2 := r.Get()
	if f2.id != 2 {
		t.Fatalf("f2.id = %d, want 2", f2.id)
	}
	release2()

	timer.advance(t)
	if !f2.isClosed() {
		t.Fatal("f2 not closed, want closed")
	}
}

func TestResource_Concurrent(t *testing.T) {
	var nextID atomic.Int64
	get := func() *fake {
		return &fake{id: nextID.Add(1)}
	}
	timer := newFakeTimer()
	r := newAfter(get, timer.after)

	// Get the first resource so we have a handle to it.
	f1, release1 := r.Get()
	if f1.id != 1 {
		t.Fatalf("f1.id = %d, want 1", f1.id)
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, release := r.Get()
			if f.id != 1 {
				t.Errorf("got id %d, want 1", f.id)
			}
			// Hold the resource for a bit to create contention.
			time.Sleep(1 * time.Millisecond)
			release()
		}()
	}
	wg.Wait()

	// All goroutines have released. Now we release our initial hold.
	release1()

	// At this point, only the resource's own reference remains.
	if f1.isClosed() {
		t.Fatal("f1 closed, want not closed")
	}

	// Advance time to release the final reference.
	timer.advance(t)

	if !f1.isClosed() {
		t.Fatal("f1 not closed, want closed")
	}

	// Getting a new resource should give a new ID.
	f2, release2 := r.Get()
	if f2.id != 2 {
		t.Fatalf("f2.id = %d, want 2", f2.id)
	}
	release2()
}
