// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestInMemory_ScheduleAndProcess(t *testing.T) {
	ctx := context.Background()

	var processed int
	var mu sync.Mutex

	processFunc := func(ctx context.Context, path, version string) (int, error) {
		mu.Lock()
		processed++
		mu.Unlock()
		return 200, nil
	}

	q := New(ctx, 2, nil, processFunc)

	tests := []struct {
		path    string
		version string
	}{
		{"example.com/mod1", "v1.0.0"},
		{"example.com/mod2", "v1.1.0"},
		{"example.com/mod3", "v2.0.0"},
	}

	for _, tc := range tests {
		ok, err := q.ScheduleFetch(ctx, tc.path, tc.version, nil)
		if err != nil {
			t.Fatalf("ScheduleFetch(%q, %q) returned error: %v", tc.path, tc.version, err)
		}
		if !ok {
			t.Errorf("ScheduleFetch(%q, %q) = false, want true", tc.path, tc.version)
		}
	}

	q.WaitForTesting(ctx)

	if got, want := processed, len(tests); got != want {
		t.Errorf("processed tasks = %d, want %d", got, want)
	}
}

func TestInMemory_ProcessFuncError(t *testing.T) {
	ctx := context.Background()

	processFunc := func(ctx context.Context, path, version string) (int, error) {
		return 500, errors.New("simulated fetch error")
	}

	q := New(ctx, 1, nil, processFunc)

	_, err := q.ScheduleFetch(ctx, "example.com/error-mod", "v1.0.0", nil)
	if err != nil {
		t.Fatalf("ScheduleFetch returned error: %v", err)
	}

	// This should complete without panicking or deadlocking.
	q.WaitForTesting(ctx)
}

func TestInMemory_WorkerConcurrency(t *testing.T) {
	ctx := context.Background()
	workerCount := 2

	var active, max int
	var mu sync.Mutex
	var wg sync.WaitGroup

	blockCh := make(chan struct{})

	processFunc := func(ctx context.Context, path, version string) (int, error) {
		mu.Lock()
		active++
		if active > max {
			max = active
		}
		mu.Unlock()

		<-blockCh

		mu.Lock()
		active--
		mu.Unlock()
		wg.Done()
		return 200, nil
	}

	q := New(ctx, workerCount, nil, processFunc)

	taskCount := 5
	wg.Add(taskCount)
	for range taskCount {
		_, err := q.ScheduleFetch(ctx, "example.com/concurrent", "v1.0.0", nil)
		if err != nil {
			t.Fatalf("ScheduleFetch returned error: %v", err)
		}
	}

	close(blockCh)
	wg.Wait()
	q.WaitForTesting(ctx)

	if max > workerCount {
		t.Errorf("max concurrent workers = %d, want <= %d", max, workerCount)
	}
}
