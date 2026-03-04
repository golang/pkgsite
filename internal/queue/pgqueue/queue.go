// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgqueue provides a Postgres-backed queue implementation for
// scheduling and processing fetch actions. It supports multiple concurrent
// workers (processes or goroutines)
package pgqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/queue"
)

// The frequency at which we poll for work.
const pollInterval = 5 * time.Second

// ProcessFunc is the function signature for processing dequeued work.
type ProcessFunc func(ctx context.Context, modulePath, version string) (int, error)

// Queue implements the Queue interface backed by a Postgres table. It is safe
// for concurrent use by multiple goroutines and processes.
type Queue struct {
	db *database.DB
}

// New creates the queue_tasks table if it doesn't exist and returns a Queue.
func New(ctx context.Context, db *database.DB) (*Queue, error) {
	// TODO(jbarkhuysen): If we find it onerous to do table updates over time, we
	// may want to consider alternatives to doing this here.
	if _, err := db.Exec(ctx, createTableQuery); err != nil {
		return nil, fmt.Errorf("pgqueue.New: creating table: %w", err)
	}
	return &Queue{db: db}, nil
}

const createTableQuery = `
CREATE TABLE IF NOT EXISTS queue_tasks (
    id          BIGSERIAL PRIMARY KEY,
    task_name   TEXT UNIQUE NOT NULL,
    module_path TEXT NOT NULL,
    version     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_queue_tasks_started_created
ON queue_tasks (started_at, created_at);`

// ScheduleFetch inserts a task into queue_tasks. It returns (true, nil) if the
// task was inserted, or (false, nil) if it was a duplicate.
func (q *Queue) ScheduleFetch(ctx context.Context, modulePath, version string, opts *queue.Options) (bool, error) {
	taskName := modulePath + "@" + version
	if opts != nil && opts.Suffix != "" {
		taskName += "-" + opts.Suffix
	}
	n, err := q.db.Exec(ctx,
		`INSERT INTO queue_tasks (task_name, module_path, version) VALUES ($1, $2, $3) ON CONFLICT (task_name) DO NOTHING`,
		taskName, modulePath, version)
	if err != nil {
		return false, fmt.Errorf("pgqueue.ScheduleFetch(%q, %q): %w", modulePath, version, err)
	}
	return n == 1, nil
}

// Poll starts background polling for work. It spawns the given number of worker
// goroutines, each of which periodically claims a task, runs processFunc, and
// deletes the task on completion. It blocks until ctx is cancelled.
func (q *Queue) Poll(ctx context.Context, workers int, processFunc ProcessFunc) {
	wg := sync.WaitGroup{}
	for range workers {
		wg.Go(func() {
			// Periodically claim work.
			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					q.claimAndProcess(ctx, processFunc)
				}
			}
		})
	}
	wg.Wait()
}

// TODO(jbarkhuysen): 5m stall timeout is baked in; we might want to make it
// variable in the future.
const dequeueQuery = `
WITH next_task AS (
    SELECT id
    FROM queue_tasks
    WHERE started_at IS NULL
       OR started_at + INTERVAL '5 minutes' < NOW()
    ORDER BY created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE queue_tasks
SET started_at = NOW()
WHERE id = (SELECT id FROM next_task)
RETURNING id, module_path, version, started_at`

func (q *Queue) claimAndProcess(ctx context.Context, processFunc ProcessFunc) {
	var id int64
	var modulePath, version string
	var startedAt time.Time
	err := q.db.QueryRow(ctx, dequeueQuery).Scan(&id, &modulePath, &version, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return // There's no work: no-op.
	}
	if err != nil {
		log.Errorf(ctx, "pgqueue: dequeue: %v", err)
		return
	}

	log.Infof(ctx, "pgqueue: processing %s@%s (task %d)", modulePath, version, id)
	code, err := processFunc(ctx, modulePath, version)
	if err != nil {
		log.Errorf(ctx, "pgqueue: processing %s@%s: status=%d err=%v", modulePath, version, code, err)
		// This still gets removed (delete below) so that we don't endlessly
		// fail the same work item.
	}

	// Use a background context for cleanup so the delete succeeds even if
	// the poll context has been cancelled.
	delCtx := context.Background()
	if _, err := q.db.Exec(delCtx, `DELETE FROM queue_tasks WHERE id = $1 AND started_at = $2`, id, startedAt); err != nil {
		log.Errorf(delCtx, "pgqueue: deleting task %d: %v", id, err)
	}
}
