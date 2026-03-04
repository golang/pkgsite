// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pgqueue

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"

	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/queue"
)

const testDBName = "discovery_pgqueue_test"

var testDB *database.DB

func TestMain(m *testing.M) {
	if os.Getenv("GO_DISCOVERY_TESTDB") != "true" {
		log.Printf("SKIPPING: GO_DISCOVERY_TESTDB is not set to true")
		return
	}
	if err := database.CreateDBIfNotExists(testDBName); err != nil {
		if errors.Is(err, derrors.NotFound) {
			log.Printf("SKIPPING: could not connect to DB: %v", err)
			return
		}
		log.Fatal(err)
	}
	db, err := database.Open("pgx", database.DBConnURI(testDBName), "test")
	if err != nil {
		log.Fatal(err)
	}
	testDB = db
	os.Exit(m.Run())
}

func setup(t *testing.T) *Queue {
	t.Helper()
	ctx := context.Background()
	if testDB == nil {
		t.Skip("test database not available")
	}
	// Drop and recreate the table for a clean slate.
	if _, err := testDB.Exec(ctx, `DROP TABLE IF EXISTS queue_tasks`); err != nil {
		t.Fatal(err)
	}
	q, err := New(ctx, testDB)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestScheduleFetch(t *testing.T) {
	q := setup(t)
	ctx := context.Background()

	enqueued, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !enqueued {
		t.Error("ScheduleFetch() = false, want true")
	}

	// Same module@version should be deduplicated.
	enqueued, err = q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if enqueued {
		t.Error("ScheduleFetch() duplicate = true, want false")
	}
}

func TestScheduleFetchWithSuffix(t *testing.T) {
	q := setup(t)
	ctx := context.Background()

	if _, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil); err != nil {
		t.Fatal(err)
	}

	// Same module@version with a suffix should not be deduplicated.
	enqueued, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", &queue.Options{Suffix: "reprocess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !enqueued {
		t.Error("ScheduleFetch() with suffix = false, want true")
	}
}

func TestPollProcessesTasks(t *testing.T) {
	q := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ScheduleFetch(ctx, "golang.org/x/net", "v0.1.0", nil); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var processed []string

	go q.Poll(ctx, 2, func(ctx context.Context, modulePath, version string) (int, error) {
		mu.Lock()
		processed = append(processed, modulePath+"@"+version)
		mu.Unlock()
		return 200, nil
	})

	// Wait for tasks to be processed.
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for tasks to be processed")
		case <-time.After(100 * time.Millisecond):
			mu.Lock()
			n := len(processed)
			mu.Unlock()
			if n == 2 {
				cancel()
				return
			}
		}
	}
}

func TestPollDeletesTaskOnError(t *testing.T) {
	q := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		q.Poll(ctx, 1, func(ctx context.Context, modulePath, version string) (int, error) {
			close(done)
			return 500, errors.New("something went wrong")
		})
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for task to be processed")
	}
	cancel()

	// Verify the task was deleted despite the error.
	var count int
	err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM queue_tasks`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	// Allow a brief moment for the delete to complete.
	time.Sleep(100 * time.Millisecond)
	err = testDB.QueryRow(context.Background(), `SELECT count(*) FROM queue_tasks`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("queue_tasks count = %d, want 0", count)
	}
}

func TestPollReclaimsStalledTasks(t *testing.T) {
	q := setup(t)
	ctx := context.Background()

	if _, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil); err != nil {
		t.Fatal(err)
	}

	// Simulate a stalled task by setting started_at to the past.
	if _, err := testDB.Exec(ctx,
		`UPDATE queue_tasks SET started_at = NOW() - INTERVAL '10 minutes'`); err != nil {
		t.Fatal(err)
	}

	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	go func() {
		q.Poll(pollCtx, 1, func(ctx context.Context, modulePath, version string) (int, error) {
			close(done)
			return 200, nil
		})
	}()

	select {
	case <-done:
		// Task was reclaimed and processed.
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for stalled task to be reclaimed")
	}
	cancel()
}

func TestStalledWorkerDeleteIsNoop(t *testing.T) {
	// Worker1 claims a task, but stalls. Worker2 claims it later. Worker1
	// unstalls and completes, but does not delete the task from the queue: once
	// worker2 has the task, only it may do the delete. It unstalls and finishes
	// and we assert the task is deleted from the queue.
	q := setup(t)
	ctx := context.Background()

	if _, err := q.ScheduleFetch(ctx, "golang.org/x/text", "v0.3.0", nil); err != nil {
		t.Fatal(err)
	}

	worker1Claimed := make(chan struct{})
	worker1Stall := make(chan struct{})
	worker1Done := make(chan struct{})
	worker2Claimed := make(chan struct{})
	worker2Stall := make(chan struct{})
	worker2Done := make(chan struct{})

	// Worker 1 claims the task and stalls.
	go func() {
		q.claimAndProcess(ctx, func(ctx context.Context, modulePath, version string) (int, error) {
			close(worker1Claimed)
			<-worker1Stall
			return 200, nil
		})
		close(worker1Done)
	}()

	// Wait for worker 1 to enter processFunc, then backdate started_at so the
	// task is eligible for reclaim.
	<-worker1Claimed
	if _, err := testDB.Exec(ctx, `UPDATE queue_tasks SET started_at = NOW() - INTERVAL '10 minutes'`); err != nil {
		t.Fatal(err)
	}

	// Worker 2 reclaims the task and stalls.
	go func() {
		q.claimAndProcess(ctx, func(ctx context.Context, modulePath, version string) (int, error) {
			close(worker2Claimed)
			<-worker2Stall
			return 200, nil
		})
		close(worker2Done)
	}()
	<-worker2Claimed

	// Unstall worker 1. Its delete should be a no-op because worker 2 has a
	// newer started_at.
	close(worker1Stall)
	<-worker1Done

	var count int
	if err := testDB.QueryRow(ctx, `SELECT count(*) FROM queue_tasks`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("after worker 1: task count = %d, want 1 (worker 1 should not have deleted it)", count)
	}

	// Unstall worker 2. Its delete should succeed.
	close(worker2Stall)
	<-worker2Done

	if err := testDB.QueryRow(ctx, `SELECT count(*) FROM queue_tasks`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("after worker 2: task count = %d, want 0", count)
	}
}
