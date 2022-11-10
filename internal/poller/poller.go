// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package poller supports periodic polling to load a value.
package poller

import (
	"context"
	"sync"
	"time"
)

// A Getter returns a value.
type Getter func(context.Context) (any, error)

// A Poller maintains a current value, and refreshes it by periodically
// polling for a new value.
type Poller struct {
	getter  Getter
	onError func(error)
	mu      sync.Mutex
	current any
}

// New creates a new poller with an initial value. The getter is invoked
// to obtain updated values. Errors returned from the getter are passed
// to onError.
func New(initial any, getter Getter, onError func(error)) *Poller {
	return &Poller{
		getter:  getter,
		onError: onError,
		current: initial,
	}
}

// Start begins polling in a separate goroutine, at the given period. To stop
// the goroutine, cancel the context passed to Start.
func (p *Poller) Start(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)

	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				ctx2, cancel := context.WithTimeout(ctx, period)
				p.Poll(ctx2)
				cancel()
			}
		}
	}()
}

// Poll calls p's getter immediately and synchronously.
func (p *Poller) Poll(ctx context.Context) {
	next, err := p.getter(ctx)
	if err != nil {
		p.onError(err)
	} else {
		p.mu.Lock()
		p.current = next
		p.mu.Unlock()
	}
}

// Current returns the current value. Initially, this is the value passed to New.
// After each successful poll, the value is updated.
// If a poll fails, the value remains unchanged.
func (p *Poller) Current() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}
