// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lru provides an LRU cache.
package lru

import (
	"fmt"
	"math"
	"sync"
)

// Cache is an LRU cache.
type Cache[K comparable, V any] struct {
	mu      sync.Mutex
	size    int
	entries map[K]*entry[V]
	tick    uint // increases every time an entry is used
}

type entry[V any] struct {
	lastUsed uint // the tick of the last operation
	v        V
}

// New returns a new Cache. Size must be positive or it will panic.
func New[K comparable, V any](size int) *Cache[K, V] {
	if size < 1 {
		panic(fmt.Errorf("lru.New called with non-positive size %v", size))
	}
	return &Cache[K, V]{
		size:    size,
		entries: map[K]*entry[V]{},
	}
}

// Get gets the entry for k in the Cache.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[k]
	if !ok {
		var zero V
		return zero, false
	}
	c.tick++
	entry.lastUsed = c.tick
	return entry.v, true
}

// Put puts in an entry for k, v in Cache, evicting
// the least recently used entry if necessary.
func (c *Cache[K, V]) Put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[k]
	if !ok {
		// k not already in c.entries. We need to evict the least recently
		// used entry.
		if c.size < 1 {
			panic("attempting to insert into an uninitialized cache.")
		}
		if len(c.entries) > c.size {
			panic(fmt.Errorf("size of cache, %d, has grown beyond size limit %d", len(c.entries), c.size))
		}
		if len(c.entries) == c.size {
			// evict least recently used element.
			var oldestTick uint = math.MaxUint
			var oldestKey K
			for k, e := range c.entries {
				if e.lastUsed <= oldestTick {
					oldestTick = e.lastUsed
					oldestKey = k
				}
			}
			delete(c.entries, oldestKey)
		}
	}
	c.tick++
	c.entries[k] = &entry[V]{lastUsed: c.tick, v: v}
}
