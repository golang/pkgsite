// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides a queue interface that can be used for
// asynchronous scheduling of fetch actions.
package queue

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
)

// A Queue provides an interface for asynchronous scheduling of fetch actions.
type Queue interface {
	ScheduleFetch(ctx context.Context, modulePath, version string, opts *Options) (bool, error)
}

// Options is used to provide option arguments for a task queue.
type Options struct {
	// DisableProxyFetch reports whether proxyfetch should be set to off when
	// making a fetch request.
	DisableProxyFetch bool

	// Suffix is used to force reprocessing of tasks that would normally be
	// de-duplicated. It is appended to the task name.
	Suffix string

	// Source is the source that requested the task to be queued. It is
	// either "frontend" or the empty string if it is the worker.
	Source string
}

const (
	DisableProxyFetchParam = "proxyfetch"
	DisableProxyFetchValue = "off"
	SourceParam            = "source"
	SourceFrontendValue    = "frontend"
	SourceWorkerValue      = "worker"
)

// InMemory is a Queue implementation that schedules in-process fetch
// operations. Unlike the GCP task queue, it will not automatically retry tasks
// on failure.
//
// This should only be used for local development.
type InMemory struct {
	queue       chan internal.Modver
	done        chan struct{}
	experiments []string
}

type InMemoryProcessFunc func(context.Context, string, string) (int, error)

// NewInMemory creates a new InMemory that asynchronously fetches
// from proxyClient and stores in db. It uses workerCount parallelism to
// execute these fetches.
func NewInMemory(ctx context.Context, workerCount int, experiments []string, processFunc InMemoryProcessFunc) *InMemory {
	q := &InMemory{
		queue:       make(chan internal.Modver, 1000),
		experiments: experiments,
		done:        make(chan struct{}),
	}
	sem := make(chan struct{}, workerCount)
	go func() {
		for v := range q.queue {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			// If a worker is available, make a request to the fetch service inside a
			// goroutine and wait for it to finish.
			go func(v internal.Modver) {
				defer func() { <-sem }()

				log.Infof(ctx, "Fetch requested: %s (workerCount = %d)", v, cap(sem))

				fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				fetchCtx = experiment.NewContext(fetchCtx, experiments...)
				defer cancel()

				if _, err := processFunc(fetchCtx, v.Path, v.Version); err != nil {
					log.Error(fetchCtx, err)
				}
			}(v)
		}
		for i := 0; i < cap(sem); i++ {
			select {
			case <-ctx.Done():
				panic(fmt.Sprintf("InMemory queue context done: %v", ctx.Err()))
			case sem <- struct{}{}:
			}
		}
		close(q.done)
	}()
	return q
}

// ScheduleFetch pushes a fetch task into the local queue to be processed
// asynchronously.
func (q *InMemory) ScheduleFetch(ctx context.Context, modulePath, version string, _ *Options) (bool, error) {
	q.queue <- internal.Modver{Path: modulePath, Version: version}
	return true, nil
}

// WaitForTesting waits for all queued requests to finish. It should only be
// used by test code.
func (q *InMemory) WaitForTesting(ctx context.Context) {
	close(q.queue)
	<-q.done
}
