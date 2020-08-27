// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides queue implementations that can be used for
// asynchronous scheduling of fetch actions.
package queue

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A Queue provides an interface for asynchronous scheduling of fetch actions.
type Queue interface {
	ScheduleFetch(ctx context.Context, modulePath, version, suffix string, taskIDChangeInterval time.Duration) error
}

// New creates a new Queue with name queueName based on the configuration
// in cfg. When running locally, Queue uses numWorkers concurrent workers.
func New(ctx context.Context, cfg *config.Config, queueName string, numWorkers int, db *postgres.DB, processFunc inMemoryProcessFunc) (Queue, error) {
	if !cfg.OnGCP() {
		experiments, err := db.GetExperiments(ctx)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range experiments {
			if e.Rollout > 0 {
				names = append(names, e.Name)
			}
		}
		return NewInMemory(ctx, numWorkers, names, processFunc), nil
	}

	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	if queueName == "" {
		return nil, errors.New("missing queue name: queueName cannot be empty")
	}
	return newGCP(cfg, client, queueName), nil
}

// GCP provides a Queue implementation backed by the Google Cloud Tasks
// API.
type GCP struct {
	cfg     *config.Config
	client  *cloudtasks.Client
	queueID string
}

// NewGCP returns a new Queue that can be used to enqueue tasks using the
// cloud tasks API.  The given queueID should be the name of the queue in the
// cloud tasks console.
func newGCP(cfg *config.Config, client *cloudtasks.Client, queueID string) *GCP {
	return &GCP{
		cfg:     cfg,
		client:  client,
		queueID: queueID,
	}
}

// ScheduleFetch enqueues a task on GCP to fetch the given modulePath and
// version. It returns an error if there was an error hashing the task name, or
// an error pushing the task to GCP.
func (q *GCP) ScheduleFetch(ctx context.Context, modulePath, version, suffix string, taskIDChangeInterval time.Duration) (err error) {
	// the new taskqueue API requires a deadline of <= 30s
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer derrors.Wrap(&err, "queue.ScheduleFetch(%q, %q, %q, %d)", modulePath, version, suffix, taskIDChangeInterval)

	req := q.newTaskRequest(modulePath, version, suffix, taskIDChangeInterval)
	if _, err := q.client.CreateTask(ctx, req); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			log.Infof(ctx, "ignoring duplicate task ID %s: %s@%s", req.Task.Name, modulePath, version)
		} else {
			return fmt.Errorf("q.client.CreateTask(ctx, req): %v", err)
		}
	}
	return nil
}

func (q *GCP) newTaskRequest(modulePath, version, suffix string, taskIDChangeInterval time.Duration) *taskspb.CreateTaskRequest {
	queueName := fmt.Sprintf("projects/%s/locations/%s/queues/%s", q.cfg.ProjectID, q.cfg.LocationID, q.queueID)
	mod := fmt.Sprintf("%s/@v/%s", modulePath, version)
	u := fmt.Sprintf("/fetch/" + mod)
	taskID := newTaskID(modulePath, version, time.Now(), taskIDChangeInterval)
	req := &taskspb.CreateTaskRequest{
		Parent: queueName,
		Task: &taskspb.Task{
			Name: fmt.Sprintf("%s/tasks/%s", queueName, taskID),
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod:  taskspb.HttpMethod_POST,
					RelativeUri: u,
					AppEngineRouting: &taskspb.AppEngineRouting{
						Service: q.cfg.QueueService,
					},
				},
			},
		},
	}
	// If suffix is non-empty, append it to the task name. This lets us force reprocessing
	// of tasks that would normally be de-duplicated.
	if suffix != "" {
		req.Task.Name += "-" + suffix
	}
	return req
}

// Create a task ID for the given module path and version.
// Task IDs can contain only letters ([A-Za-z]), numbers ([0-9]), hyphens (-), or underscores (_).
// Also include a truncated time in the hash, so it changes periodically.
//
// Since we truncate the time to the nearest taskIDChangeInterval, it's still possible
// for two identical tasks to appear within that time period (for example, one at 2:59
// and the other at 3:01) -- each is part of a different taskIDChangeInterval-sized chunk
// of time. But there will never be a third identical task in that interval.
func newTaskID(modulePath, version string, now time.Time, taskIDChangeInterval time.Duration) string {
	t := now.Truncate(taskIDChangeInterval)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(modulePath+"@"+version+"-"+t.String())))
}

type moduleVersion struct {
	modulePath, version string
}

// InMemory is a Queue implementation that schedules in-process fetch
// operations. Unlike the GCP task queue, it will not automatically retry tasks
// on failure.
//
// This should only be used for local development.
type InMemory struct {
	queue       chan moduleVersion
	sem         chan struct{}
	experiments []string
}

type inMemoryProcessFunc func(context.Context, string, string) (int, error)

// NewInMemory creates a new InMemory that asynchronously fetches
// from proxyClient and stores in db. It uses workerCount parallelism to
// execute these fetches.
func NewInMemory(ctx context.Context, workerCount int, experiments []string, processFunc inMemoryProcessFunc) *InMemory {
	q := &InMemory{
		queue:       make(chan moduleVersion, 1000),
		sem:         make(chan struct{}, workerCount),
		experiments: experiments,
	}
	go func() {
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

				log.Infof(ctx, "Fetch requested: %q %q (workerCount = %d)", v.modulePath, v.version, cap(q.sem))

				fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				fetchCtx = experiment.NewContext(fetchCtx, experiments...)
				defer cancel()

				if _, err := processFunc(fetchCtx, v.modulePath, v.version); err != nil {
					log.Error(fetchCtx, err)
				}
			}(v)
		}
	}()
	return q
}

// ScheduleFetch pushes a fetch task into the local queue to be processed
// asynchronously.
func (q *InMemory) ScheduleFetch(ctx context.Context, modulePath, version, suffix string, taskIDChangeInterval time.Duration) error {
	q.queue <- moduleVersion{modulePath, version}
	return nil
}

// WaitForTesting waits for all queued requests to finish. It should only be
// used by test code.
func (q InMemory) WaitForTesting(ctx context.Context) {
	for i := 0; i < cap(q.sem); i++ {
		select {
		case <-ctx.Done():
			return
		case q.sem <- struct{}{}:
		}
	}
	close(q.queue)
}
