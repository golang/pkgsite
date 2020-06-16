// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker

import (
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBucketReset(t *testing.T) {
	bu := bucket{12, 8}
	bu.reset()
	if bu.successes != 0 {
		t.Errorf("got successes = %d, want %d", bu.successes, 0)
	}
	if bu.failures != 0 {
		t.Errorf("got failures = %d, want %d", bu.failures, 0)
	}
}

func TestResetCounts(t *testing.T) {
	b := newTestBreaker(Config{})
	for i := 0; i < len(b.buckets); i++ {
		b.buckets[i].successes = 10
		b.buckets[i].failures = 15
	}

	b.resetCounts()
	testBuckets(t, b.buckets[:], 0, 0)
	testCounts(t, b, 0, 0, 0)
}

func TestNewBreaker(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2020, time.May, 26, 18, 0, 0, 0, time.UTC)
	}
	got, err := New(Config{
		FailsToRed:       10,
		FailureThreshold: 0.65,
		GreenInterval:    20 * time.Second,
		MinTimeout:       30 * time.Second,
		MaxTimeout:       16 * time.Minute,
		SuccsToGreen:     15,
	})
	if err != nil {
		t.Fatalf("New() returned %e, want nil", err)
	}
	want := &Breaker{
		config: Config{
			FailsToRed:       10,
			FailureThreshold: 0.65,
			GreenInterval:    20 * time.Second,
			MinTimeout:       30 * time.Second,
			MaxTimeout:       16 * time.Minute,
			SuccsToGreen:     15,
		},
		buckets:          [numBuckets]bucket{},
		granularity:      2500 * time.Millisecond,
		state:            Green,
		cur:              0,
		consecutiveSuccs: 0,
		timeout:          30 * time.Second,
		lastEvent:        time.Date(2020, time.May, 26, 18, 0, 0, 0, time.UTC),
	}

	diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(sync.Mutex{}), cmp.AllowUnexported(Breaker{}, bucket{}))
	if diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestIllegalBreaker(t *testing.T) {
	for _, test := range []struct {
		name   string
		config Config
	}{
		{
			name: "FailsToRed cannot be 0",
			config: Config{
				FailsToRed:       0,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "FailsToRed cannot be negative",
			config: Config{
				FailsToRed:       -5,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "FailureThreshold cannot be 0",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "FailureThreshold cannot be negative",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: -0.8,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "FailureThreshold cannot exceed 1",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 1.2,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "GreenInterval cannot be 0",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    0,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "GreenInterval cannot be negative",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    -4 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "MinTimeout cannot be 0",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       0,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "MinTimeout cannot be negative",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       -2 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "MaxTimeout cannot be 0",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       0,
				SuccsToGreen:     15,
			},
		},
		{
			name: "MaxTimeout cannot be negative",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       -12 * time.Minute,
				SuccsToGreen:     15,
			},
		},
		{
			name: "SuccsToGreen cannot be 0",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     0,
			},
		},
		{
			name: "SuccsToGreen cannot be negative",
			config: Config{
				FailsToRed:       8,
				FailureThreshold: 0.65,
				GreenInterval:    20 * time.Second,
				MinTimeout:       30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     -7,
			},
		},
		{
			name: "multiple illegal values return error",
			config: Config{
				FailsToRed:       0,
				FailureThreshold: 1.4,
				GreenInterval:    20 * time.Second,
				MinTimeout:       -30 * time.Second,
				MaxTimeout:       16 * time.Minute,
				SuccsToGreen:     100,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, err := New(test.config)
			if err == nil {
				t.Fatalf("New() returned nil error")
			}
			if b != nil {
				t.Fatalf("New() returned %+v, want nil", b)
			}
		})
	}
}

func TestBreakerGranularity(t *testing.T) {
	for _, test := range []struct {
		config Config
		want   time.Duration
	}{
		{
			config: Config{},
			want:   1250 * time.Millisecond,
		},
		{
			config: Config{GreenInterval: 1 * time.Second},
			want:   125 * time.Millisecond,
		},
		{
			config: Config{GreenInterval: 3 * time.Second},
			want:   375 * time.Millisecond,
		},
		{
			config: Config{GreenInterval: 1 * time.Minute},
			want:   7500 * time.Millisecond,
		},
		{
			config: Config{GreenInterval: 1 * time.Hour},
			want:   450 * time.Second,
		},
	} {
		b := newTestBreaker(test.config)
		if b.granularity != test.want {
			t.Errorf("b.granularity = %d, want %d", b.granularity, test.want)
		}
	}
}

func TestState(t *testing.T) {
	for _, want := range []State{
		Green,
		Yellow,
		Red,
	} {
		b := newTestBreaker(Config{})
		b.state = want
		if got := b.checkState(); got != want {
			t.Errorf("b.checkState() = %s, got %s", got, want)
		}
		if got := b.State(); got != want {
			t.Errorf("b.State() = %s, want %s", got, want)
		}
	}
}
func TestAllow(t *testing.T) {
	for _, test := range []struct {
		state       State
		shouldAllow bool
	}{
		{
			state:       Green,
			shouldAllow: true,
		},
		{
			state:       Yellow,
			shouldAllow: true,
		},
		{
			state:       Red,
			shouldAllow: false,
		},
	} {
		b := newTestBreaker(Config{})
		b.state = test.state
		allowed := b.Allow()
		if allowed != test.shouldAllow {
			t.Errorf("b.Allow() = %t in %s, want %t", allowed, test.state, test.shouldAllow)
		}
	}
}

func TestSuccesses(t *testing.T) {
	b := newTestBreaker(Config{})
	b.succeeded()
	testCounts(t, b, 1, 1, 0)
	b.succeeded()
	testCounts(t, b, 2, 2, 0)
	b.succeeded()
	testCounts(t, b, 3, 3, 0)
}

func TestFailures(t *testing.T) {
	b := newTestBreaker(Config{})
	b.failed()
	testCounts(t, b, 0, 0, 1)
	b.failed()
	testCounts(t, b, 0, 0, 2)
	b.failed()
	testCounts(t, b, 0, 0, 3)
}

func TestSucceededAndFailed(t *testing.T) {
	b := newTestBreaker(Config{})
	b.succeeded()
	testCounts(t, b, 1, 1, 0)
	b.failed()
	testCounts(t, b, 0, 1, 1)
	b.failed()
	testCounts(t, b, 0, 1, 2)
	b.succeeded()
	testCounts(t, b, 1, 2, 2)
	b.succeeded()
	testCounts(t, b, 2, 3, 2)
	b.succeeded()
	testCounts(t, b, 3, 4, 2)
	b.failed()
	testCounts(t, b, 0, 4, 3)
}

func TestUpdate(t *testing.T) {
	now := time.Now()
	b := newTestBreaker(Config{})
	b.lastEvent = now
	b.granularity = 1 * time.Second
	for i := 0; i < len(b.buckets); i++ {
		b.buckets[i].successes = 4
		b.buckets[i].failures = 9
	}

	// Update 0 buckets.
	b.update(now.Add(-1 * time.Second))
	if b.cur != 0 {
		t.Errorf("cur: got %d, want %d", b.cur, 0)
	}
	testBuckets(t, b.buckets[:], 4, 9)
	testCounts(t, b, 0, 4*len(b.buckets), 9*len(b.buckets))

	// Update next 3 buckets.
	b.update(now.Add(3 * time.Second))
	if b.cur != 3 {
		t.Errorf("cur: got %d, want %d", b.cur, 3)
	}
	testBuckets(t, b.buckets[:1], 4, 9)
	testBuckets(t, b.buckets[1:4], 0, 0)
	testBuckets(t, b.buckets[4:], 4, 9)
	testCounts(t, b, 0, 4*len(b.buckets)-12, 9*len(b.buckets)-27)

	// Update all buckets.
	b.update(now.Add(1003 * time.Second))
	expectedCur := 0
	if b.cur != expectedCur {
		t.Errorf("cur: got %d, want %d", b.cur, expectedCur)
	}
	testBuckets(t, b.buckets[:], 0, 0)
	testCounts(t, b, 0, 0, 0)
}

func TestStateChanges(t *testing.T) {
	for _, test := range []struct {
		name         string
		config       Config
		preSuccesses int
		preFailures  int
		fromState    State
		allow        bool
		success      bool
		sleep        time.Duration
		toState      State
	}{
		{
			name:         "breaker state remains green when FailsToRed is not exceeded",
			config:       Config{FailsToRed: 8, FailureThreshold: 0.5},
			preSuccesses: 6,
			preFailures:  7,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Green,
		},
		{
			name:         "breaker state remains green when FailureThreshold is not exceeded",
			config:       Config{FailsToRed: 2, FailureThreshold: 0.8},
			preSuccesses: 3,
			preFailures:  6,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Green,
		},
		{
			name:         "breaker state remains green when failure ratio = FailureThreshold",
			config:       Config{FailsToRed: 10, FailureThreshold: 0.5},
			preSuccesses: 20,
			preFailures:  19,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Green,
		},
		{
			name:         "breaker state remains green when failures = FailsToRed",
			config:       Config{FailsToRed: 10, FailureThreshold: 0.3},
			preSuccesses: 10,
			preFailures:  9,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Green,
		},
		{
			name:         "breaker state changes to red when FailureThreshold is exceeded and after FailsToRed has been exceeded",
			config:       Config{FailsToRed: 10, FailureThreshold: 0.5},
			preSuccesses: 20,
			preFailures:  20,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Red,
		},
		{
			name:         "breaker state changes to red when FailsToRed is exceeded and after FailureThreshold has been exceeded",
			config:       Config{FailsToRed: 20, FailureThreshold: 0.3},
			preSuccesses: 20,
			preFailures:  20,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Red,
		},
		{
			name:         "breaker state changes from green to red",
			config:       Config{FailsToRed: 4, FailureThreshold: 0.5},
			preSuccesses: 4,
			preFailures:  4,
			fromState:    Green,
			allow:        true,
			success:      false,
			toState:      Red,
		},
		{
			name:         "failure in yellow state changes breaker to red state",
			config:       Config{},
			preSuccesses: 0,
			preFailures:  0,
			fromState:    Yellow,
			allow:        true,
			success:      false,
			toState:      Red,
		},
		{
			name:         "breaker state changes from yellow to green",
			config:       Config{SuccsToGreen: 1},
			preSuccesses: 0,
			preFailures:  0,
			fromState:    Yellow,
			allow:        true,
			success:      true,
			toState:      Green,
		},
		{
			name:         "breaker state changes from red to yellow",
			config:       Config{MinTimeout: 1 * time.Second},
			preSuccesses: 0,
			preFailures:  0,
			fromState:    Red,
			sleep:        1*time.Second + 1*time.Nanosecond,
			toState:      Yellow,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Time{}
			timeNow = func() time.Time { return now }
			b := newTestBreaker(test.config)
			b.state = test.fromState
			b.buckets[0].successes = test.preSuccesses
			b.buckets[0].failures = test.preFailures

			allowed := b.Allow()
			if allowed != test.allow {
				t.Fatalf("b.Allow() = %t in %s, want %t", allowed, test.fromState, test.allow)
			}
			if test.allow {
				b.Record(test.success)
			}

			// Pseudo sleep.
			now = now.Add(test.sleep)

			if state := b.State(); state != test.toState {
				t.Errorf("b.State() = %s, want %s", state, test.toState)
			}
		})
	}
}

func TestRunningBreaker(t *testing.T) {
	now := time.Time{}
	timeNow = func() time.Time { return now }
	b := newTestBreaker(Config{
		GreenInterval: 5 * time.Second,
	})

	// The following tests happen sequentially. The tests' states depend on previous tests.
	for _, test := range []struct {
		name                 string
		firstSleep           time.Duration
		allow                bool
		secondSleep          time.Duration
		success              bool
		wantConsecutiveSuccs int
		wantSuccesses        int
		wantFailures         int
	}{
		{
			name:                 "successFunc called after a long time updates counts",
			firstSleep:           20 * time.Second,
			allow:                true,
			secondSleep:          20 * time.Second,
			success:              true,
			wantConsecutiveSuccs: 1,
			wantSuccesses:        1,
			wantFailures:         0,
		},
		{
			name:                 "success within GreenInterval updates counts correctly",
			firstSleep:           1 * time.Second,
			allow:                true,
			secondSleep:          3 * time.Second,
			success:              true,
			wantConsecutiveSuccs: 2,
			wantSuccesses:        2,
			wantFailures:         0,
		},
		{
			name:                 "success after a long time updates counts correctly",
			firstSleep:           30 * time.Second,
			allow:                true,
			secondSleep:          80 * time.Second,
			success:              true,
			wantConsecutiveSuccs: 3,
			wantSuccesses:        1,
			wantFailures:         0,
		},
		{
			name:                 "failure within GreenInterval updates counts correctly",
			firstSleep:           1 * time.Second,
			allow:                true,
			secondSleep:          3 * time.Second,
			success:              false,
			wantConsecutiveSuccs: 0,
			wantSuccesses:        1,
			wantFailures:         1,
		},
		{
			name:                 "second failure within GreenInterval updates counts correctly",
			firstSleep:           1 * time.Millisecond,
			allow:                true,
			secondSleep:          3 * time.Millisecond,
			success:              false,
			wantConsecutiveSuccs: 0,
			wantSuccesses:        1,
			wantFailures:         2,
		},
		{
			name:                 "failure after a long time updates counts correctly",
			firstSleep:           10 * time.Second,
			allow:                true,
			secondSleep:          4 * time.Minute,
			success:              false,
			wantConsecutiveSuccs: 0,
			wantSuccesses:        0,
			wantFailures:         1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			now = now.Add(test.firstSleep)
			allowed := b.Allow()

			if allowed != test.allow {
				t.Fatalf("breaker.Allow() = %t, want %t", allowed, test.allow)
			}

			now = now.Add(test.secondSleep)
			if test.allow {
				b.Record(test.success)
			}

			testCounts(t, b, test.wantConsecutiveSuccs, test.wantSuccesses, test.wantFailures)
		})
	}
}

func TestIncreaseTimeout(t *testing.T) {
	b := newTestBreaker(Config{
		MinTimeout: 1 * time.Second,
		MaxTimeout: 12 * time.Second,
	})
	b.timeout = 3 * time.Second

	b.increaseTimeout()
	testTimeouts(t, b, 6*time.Second, 1*time.Second, 12*time.Second)
	b.increaseTimeout()
	testTimeouts(t, b, 12*time.Second, 1*time.Second, 12*time.Second)
	b.increaseTimeout()
	testTimeouts(t, b, 12*time.Second, 1*time.Second, 12*time.Second)

	b.config.MaxTimeout = 14 * time.Second
	testTimeouts(t, b, 12*time.Second, 1*time.Second, 14*time.Second)
	b.increaseTimeout()
	testTimeouts(t, b, 14*time.Second, 1*time.Second, 14*time.Second)
	b.increaseTimeout()
	testTimeouts(t, b, 14*time.Second, 1*time.Second, 14*time.Second)
}

// newTestBreaker is like New, but with default values for easier testing.
func newTestBreaker(config Config) *Breaker {
	if config.FailsToRed <= 0 {
		config.FailsToRed = 10
	}
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 0.5
	}
	if config.GreenInterval <= 0 {
		config.GreenInterval = 10 * time.Second
	}
	if config.MinTimeout <= 0 {
		config.MinTimeout = 30 * time.Second
	}
	if config.MaxTimeout <= 0 {
		config.MaxTimeout = 4 * time.Minute
	}
	if config.SuccsToGreen <= 0 {
		config.SuccsToGreen = 20
	}
	b, _ := New(config)
	return b
}

func testCounts(t *testing.T, b *Breaker, consecutiveSuccs, wantSuccesses, wantFailures int) {
	if b.consecutiveSuccs != consecutiveSuccs {
		t.Errorf("b.consecutiveSuccs = %d, want %d", b.consecutiveSuccs, consecutiveSuccs)
	}
	successes, failures := b.counts()
	if successes != wantSuccesses {
		t.Errorf("successes = %d, want %d", successes, wantSuccesses)
	}
	if failures != wantFailures {
		t.Errorf("failures = %d, want %d", failures, wantFailures)
	}
}

func testBuckets(t *testing.T, buckets []bucket, successes, failures int) {
	for i, bu := range buckets {
		if bu.successes != successes {
			t.Errorf("slice bucket %d successes: got %d, want %d", i, bu.successes, successes)
		}
		if bu.failures != failures {
			t.Errorf("slice bucket %d failures: got %d, want %d", i, bu.failures, failures)
		}
	}
}

func testTimeouts(t *testing.T, b *Breaker, timeout, minTimeout, maxTimeout time.Duration) {
	if b.timeout != timeout {
		t.Errorf("b.timeout = %s, want %s", b.timeout, timeout)
	}
	if b.config.MinTimeout != minTimeout {
		t.Errorf("b.config.MinTimeout = %s, want %s", b.config.MinTimeout, minTimeout)
	}
	if b.config.MaxTimeout != maxTimeout {
		t.Errorf("b.config.MaxTimeout = %s, want %s", b.config.MaxTimeout, maxTimeout)
	}
}
