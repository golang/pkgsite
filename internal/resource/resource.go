// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The resource package defines types to track the lifecycle of transient
// resources, such as a database connection, that should be renewed at some
// fixed interval.
package resource

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type instance[T io.Closer] struct {
	refs atomic.Int64
	v    T
}

func (i *instance[T]) acquire(initial bool) (T, func()) {
	if i.refs.Add(1) <= 1 && !initial {
		panic("acquire on released instance")
	}
	return i.v, func() {
		if i.refs.Add(-1) == 0 {
			i.v.Close() // ignore error
			var zero T
			i.v = zero // aid GC
		}
	}
}

// A Resource is a container for a transient resource of type T that should be
// periodically renewed, such as a database connection.
//
// Its Get method returns an instance of the resource, along with a release
// function that the caller must invoke when it is done with the resource.
//
// The first call to Get creates a new resource instance. This instance is
// cached and returned by subsequent calls to Get for a fixed duration. After
// this duration expires, the next call to Get will create a new instance. The
// expired instance is closed once all its users have released it.
//
// A Resource is safe for concurrent use.
type Resource[T io.Closer] struct {
	get      func() T
	validFor time.Duration
	after    func(func()) // for testing; fakes time.AfterFunc(validFor, f)

	mu  sync.Mutex
	cur *instance[T]
}

// New creates a new Resource that is valid for the given duration. The get
// function is called to create a new resource instance when one is needed.
func New[T io.Closer](get func() T, validFor time.Duration) *Resource[T] {
	r := newAfter(get, func(f func()) {
		time.AfterFunc(validFor, f)
	})
	r.validFor = validFor
	return r
}

// newAfter returns a new resource with a fake after func, for testing.
func newAfter[T io.Closer](get func() T, after func(func())) *Resource[T] {
	return &Resource[T]{get: get, after: after}
}

// Get returns the current resource instance and a function to release it.
// The release function must be called when the caller is done with the
// resource.
func (r *Resource[T]) Get() (T, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil {
		r.cur = &instance[T]{v: r.get()}
		// Acquire one ref for the new instance that lasts the duration.
		//
		// This is distinct from the ref acquired below; it ensures that the
		// resource is not released until the duration has expired.
		_, release := r.cur.acquire(true)
		r.after(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			release()
			r.cur = nil
		})
	}
	return r.cur.acquire(false)
}
