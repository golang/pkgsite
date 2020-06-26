// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package breaker implements the circuit breaker pattern.
// see https://docs.microsoft.com/en-us/previous-versions/msp-n-p/dn589784(v=pandp.10).
//
// This package uses different terminologies for the state of the circuit breaker
// for better readability. Since it is unintuitive for users unfamiliar with
// circuits that a "closed" state means to "let the requests pass through" and
// "open" state means "don't let anything through", we use the colors red,
// yellow, and green instead of open, half-open, and closed, respectively.
//
// When the API is stable, this package may be factored out to an external location.
package breaker

import (
	"fmt"
	"sync"
	"time"
)

// For testing.
var timeNow = time.Now

// numBuckets is the number of buckets in the breaker's sliding window.
const numBuckets = 8

// State represents the current state the breaker is in.
type State int

// The breaker can have three states: Red, Yellow, or Green.
const (
	// Red state means that requests should not be allowed.
	Red State = iota
	// Yellow state means that certain requests may proceed with caution.
	Yellow
	// Green state means that requests are allowed to pass.
	Green
)

// String returns the string version of the state.
func (s State) String() string {
	switch s {
	case Red:
		return "red state"
	case Yellow:
		return "yellow state"
	case Green:
		return "green state"
	default:
		return "invalid state"
	}
}

// Config holds the configuration values for a breaker.
type Config struct {
	// FailsToRed is the number of failures to exceed before the breaker shifts
	// from green to red state.
	FailsToRed int
	// FailureThreshold is the failure ratio to exceed before the breaker
	// shifts from green to red state.
	FailureThreshold float64
	// GreenInterval is the length of the interval with which the breaker
	// checks for conditions to move from green to red state.
	GreenInterval time.Duration
	// MinTimeout is the minimum timeout period that the breaker stays in the
	// red state before moving to the yellow state.
	MinTimeout time.Duration
	// MaxTimeout is the maxmimum timeout period that the breaker stays in the
	// red state before moving to the yellow state.
	MaxTimeout time.Duration
	// SuccsToGreen is the number of successes required to shift from the
	// yellow state to the green state.
	SuccsToGreen int
}

// Breaker represents a circuit breaker.
//
// In the green state, the breaker remains green until it encounters a time
// window of length GreenInterval where there are more than FailsToRed failures
// and a failureRatio of more than FailureThreshold, in which case the
// state becomes red.
//
// In the red state, the breaker halts all requests and waits for a timeout period
// before shifting to the yellow state.
//
// In the yellow state, the breaker allows the first SuccsToGreen requests. If
// any of these fail, the state reverts to red. Otherwise, the state becomes
// green again.
//
// The timeout period is initially set to MinTimeout when the breaker shifts
// from green to yellow. By default, the timeout period is doubled each time
// the breaker fails to shift from the yellow state to the green state and is
// capped at MaxTimeout.
type Breaker struct {
	config Config

	// buckets represents a time sliding window, implemented as a ring buffer.
	buckets [numBuckets]bucket
	// granularity is the length of time each bucket is responsible for.
	granularity time.Duration

	mu               sync.Mutex
	state            State
	cur              int
	consecutiveSuccs int
	timeout          time.Duration
	lastEvent        time.Time
}

// New creates a Breaker with the given configuration.
func New(config Config) (*Breaker, error) {
	switch {
	case config.FailsToRed <= 0:
		return nil, fmt.Errorf("illegal value for FailsToRed")
	case config.FailureThreshold <= 0, config.FailureThreshold > 1:
		return nil, fmt.Errorf("illegal value for FailureThreshold")
	case config.GreenInterval <= 0:
		return nil, fmt.Errorf("illegal value for GreenInterval")
	case config.MinTimeout <= 0:
		return nil, fmt.Errorf("illegal value for MinTimeout")
	case config.MaxTimeout <= 0:
		return nil, fmt.Errorf("illegal value for MaxTimeout")
	case config.SuccsToGreen <= 0:
		return nil, fmt.Errorf("illegal value for SuccsToGreen")
	default:
		return &Breaker{
			config:      config,
			state:       Green,
			granularity: config.GreenInterval / numBuckets,
			timeout:     config.MinTimeout,
			lastEvent:   timeNow(),
		}, nil
	}
}

// State returns the state of the breaker.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.checkState()
}

// checkState returns the state of the breaker without obtaining a mutex lock.
// The state is updated if sufficient time has passed since the last event.
func (b *Breaker) checkState() State {
	now := timeNow()
	if b.state == Red && now.After(b.lastEvent.Add(b.timeout)) {
		b.state = Yellow
	}
	return b.state
}

// Allow reports whether an event may happen at time now. If Allow returns
// true, the user must then call Record to register whether the event succeeded.
func (b *Breaker) Allow() bool {
	return b.State() != Red
}

// Record registers the success or failure of an event with the circuit breaker.
// Use this function after a call to Allow returned true.
func (b *Breaker) Record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.update(timeNow())
	if success {
		b.succeeded()
	} else {
		b.failed()
	}
}

// succeeded signals that an allowed request has succeeded. The breaker state is
// changed if necessary.
func (b *Breaker) succeeded() {
	b.buckets[b.cur].successes++
	b.consecutiveSuccs++
	if b.checkState() == Yellow && b.consecutiveSuccs >= b.config.SuccsToGreen {
		b.state = Green
		b.timeout = b.config.MinTimeout
		b.resetCounts()
	}
}

// failed signals that an allowed request has failed. The breaker state is
// changed if necessary.
func (b *Breaker) failed() {
	b.buckets[b.cur].failures++
	b.consecutiveSuccs = 0
	switch b.checkState() {
	case Yellow:
		b.increaseTimeout()
		b.state = Red
	case Green:
		// Check conditions to move to red state.
		successes, failures := b.counts()
		totalRequests := successes + failures
		if failures <= b.config.FailsToRed || totalRequests == 0 {
			return
		}
		failureRatio := float64(failures) / float64(totalRequests)
		if failureRatio > b.config.FailureThreshold {
			b.state = Red
		}
	}
}

// update updates the values of breaker due to the passage of time.
func (b *Breaker) update(now time.Time) {
	// Ignore updates from the past.
	if now.Before(b.lastEvent) {
		return
	}
	since := now.Sub(b.lastEvent)
	b.advance(int(since / b.granularity))
	b.lastEvent = now
}

// advance advances the breaker's sliding window by n buckets. The counts of
// successes and failures are also updated to include only the buckets in the
// current window.
func (b *Breaker) advance(n int) {
	if n >= len(b.buckets) {
		b.resetCounts()
		b.cur = 0
		return
	}
	for i := 0; i < n; i++ {
		b.cur = (b.cur + 1) % len(b.buckets)
		b.buckets[b.cur].reset()
	}
}

// counts returns the total number of successes and failures in the breaker's
// sliding window.
func (b *Breaker) counts() (successes, failures int) {
	for _, bu := range b.buckets {
		successes += bu.successes
		failures += bu.failures
	}
	return successes, failures
}

// resetCounts resets all the buckets in breaker.
func (b *Breaker) resetCounts() {
	for i := range b.buckets {
		b.buckets[i].reset()
	}
}

// increaseTimeout exponentially increases the breaker's timeout period,
// capped at maxTimeout.
func (b *Breaker) increaseTimeout() {
	if 2*b.timeout <= b.config.MaxTimeout {
		b.timeout *= 2
		return
	}
	b.timeout = b.config.MaxTimeout
}

type bucket struct {
	successes int
	failures  int
}

// reset resets the values in the bucket.
func (bu *bucket) reset() {
	bu.successes = 0
	bu.failures = 0
}
