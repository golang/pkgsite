// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"google.golang.org/appengine/taskqueue"
)

// A Queue provides an interface for asynchronous scheduling of fetch actions.
type Queue interface {
	ScheduleFetch(ctx context.Context, modulePath, version string) error
}

// GCPQueue provides a Queue implementation backed by the Google Cloud Tasks
// API.
type GCPQueue struct {
	QueueName string
}

// ScheduleFetch enqueues a task on GCP to fetch the given modulePath and
// version. It returns an error if there was an error hashing the task name, or
// an error pushing the task to GCP.
func (q *GCPQueue) ScheduleFetch(ctx context.Context, modulePath, version string) error {
	if q == nil {
		return nil
	}
	u := fmt.Sprintf("/fetch/%s/@v/%s", modulePath, version)
	t := taskqueue.NewPOSTTask(u, nil)
	if _, err := taskqueue.Add(ctx, t, q.QueueName); err != nil {
		log.Printf("taskqueue.Add(ctx, t, %q): %v", q.QueueName, err)
	}

	return nil
}

type moduleVersion struct {
	modulePath, version string
}

// InMemoryQueue is a Queue implementation that schedules in-process fetch
// operations. Unlike the GCP task queue, it will not automatically retry tasks
// on failure.
//
// This should only be used for local development.
type InMemoryQueue struct {
	proxyClient *proxy.Client
	db          *postgres.DB

	queue chan moduleVersion
	sem   chan struct{}

	workerCount int
}

// NewInMemoryQueue creates a new InMemoryQueue that asynchronously fetches
// from proxyClient and stores in db. It uses workerCount parallelism to
// execute these fetches.
func NewInMemoryQueue(ctx context.Context, proxyClient *proxy.Client, db *postgres.DB, workerCount int) *InMemoryQueue {
	q := &InMemoryQueue{
		proxyClient: proxyClient,
		db:          db,
		queue:       make(chan moduleVersion, 1000),
		workerCount: workerCount,
		sem:         make(chan struct{}, workerCount),
	}
	go q.process(ctx)
	return q
}

func (q *InMemoryQueue) process(ctx context.Context) {

	for v := range q.queue {
		select {
		case <-ctx.Done():
			return
		case q.sem <- struct{}{}:
		}

		// If a worker is available, make a request to the fetch service inside a
		// goroutine and wait for it to finish.
		go func(v moduleVersion) {
			defer func() { <-q.sem }()

			log.Printf("Fetch requested: %q %q (workerCount = %d)", v.modulePath, v.version, q.workerCount)

			fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			if _, err := fetchAndUpdateState(fetchCtx, v.modulePath, v.version, q.proxyClient, q.db); err != nil {
				log.Printf("fetchAndUpdateState(ctx, %q, %q, ...): %v", v.modulePath, v.version, err)
			}
		}(v)
	}
}

// ScheduleFetch pushes a fetch task into the local queue to be processed
// asynchronously.
func (q *InMemoryQueue) ScheduleFetch(ctx context.Context, modulePath, version string) error {
	q.queue <- moduleVersion{modulePath, version}
	return nil
}

// waitForTesting waits for all queued requests to finish. It should only be
// used by test code.
func (q InMemoryQueue) waitForTesting(ctx context.Context) {
	for i := 0; i < q.workerCount; i++ {
		select {
		case <-ctx.Done():
			return
		case q.sem <- struct{}{}:
		}
	}
	close(q.queue)
}
