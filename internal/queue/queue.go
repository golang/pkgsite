// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides queue implementations that can be used for
// asynchronous scheduling of fetch actions.
package queue

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"strings"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A Queue provides an interface for asynchronous scheduling of fetch actions.
type Queue interface {
	ScheduleFetch(ctx context.Context, modulePath, version, suffix string) (bool, error)
}

// New creates a new Queue with name queueName based on the configuration
// in cfg. When running locally, Queue uses numWorkers concurrent workers.
func New(ctx context.Context, cfg *config.Config, queueName string, numWorkers int, expGetter middleware.ExperimentGetter, processFunc inMemoryProcessFunc) (Queue, error) {
	if !cfg.OnGCP() {
		experiments, err := expGetter(ctx)
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
	g, err := newGCP(cfg, client, queueName)
	if err != nil {
		return nil, err
	}
	log.Infof(ctx, "enqueuing at %s with queueService=%q, queueURL=%q", g.queueName, g.queueService, g.queueURL)
	return g, nil
}

// GCP provides a Queue implementation backed by the Google Cloud Tasks
// API.
type GCP struct {
	client       *cloudtasks.Client
	queueName    string // full GCP name of the queue
	queueService string // AppEngine service to post tasks to
	queueURL     string // non-AppEngine URL to post tasks to
	// token holds information that lets the task queue construct an authorized request to the worker.
	// Since the worker sits behind the IAP, the queue needs an identity token that includes the
	// identity of a service account that has access, and the client ID for the IAP.
	// We use the service account of the current process.
	token *taskspb.HttpRequest_OidcToken
}

// NewGCP returns a new Queue that can be used to enqueue tasks using the
// cloud tasks API.  The given queueID should be the name of the queue in the
// cloud tasks console.
func newGCP(cfg *config.Config, client *cloudtasks.Client, queueID string) (_ *GCP, err error) {
	defer derrors.Wrap(&err, "newGCP(cfg, client, %q)", queueID)
	if queueID == "" {
		return nil, errors.New("empty queueID")
	}
	if cfg.ProjectID == "" {
		return nil, errors.New("empty ProjectID")
	}
	if cfg.LocationID == "" {
		return nil, errors.New("empty LocationID")
	}
	if cfg.QueueService == "" && cfg.QueueURL == "" {
		return nil, errors.New("both QueueService and QueueURL are empty")
	}
	if cfg.QueueService != "" && cfg.QueueURL != "" {
		return nil, errors.New("both  QueueService and QueueURL are non-empty")
	}
	if cfg.OnAppEngine() && cfg.QueueService == "" {
		return nil, errors.New("on AppEngine, but QueueService is empty")
	}
	if cfg.QueueURL != "" {
		if cfg.ServiceAccount == "" {
			return nil, errors.New("need ServiceAccount with QueueURL")
		}
		if cfg.QueueAudience == "" {
			return nil, errors.New("need QueueAudience with QueueURL")
		}
	}
	return &GCP{
		client:       client,
		queueName:    fmt.Sprintf("projects/%s/locations/%s/queues/%s", cfg.ProjectID, cfg.LocationID, queueID),
		queueService: cfg.QueueService,
		queueURL:     cfg.QueueURL,
		token: &taskspb.HttpRequest_OidcToken{
			OidcToken: &taskspb.OidcToken{
				ServiceAccountEmail: cfg.ServiceAccount,
				Audience:            cfg.QueueAudience,
			},
		},
	}, nil
}

// ScheduleFetch enqueues a task on GCP to fetch the given modulePath and
// version. It returns an error if there was an error hashing the task name, or
// an error pushing the task to GCP. If the task was a duplicate, it returns (false, nil).
func (q *GCP) ScheduleFetch(ctx context.Context, modulePath, version, suffix string) (enqueued bool, err error) {
	// the new taskqueue API requires a deadline of <= 30s
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer derrors.Wrap(&err, "queue.ScheduleFetch(%q, %q, %q)", modulePath, version, suffix)

	req := q.newTaskRequest(modulePath, version, suffix)
	enqueued = true
	if _, err := q.client.CreateTask(ctx, req); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			log.Debugf(ctx, "ignoring duplicate task ID %s: %s@%s", req.Task.Name, modulePath, version)
			enqueued = false
		} else {
			return false, fmt.Errorf("q.client.CreateTask(ctx, req): %v", err)
		}
	}
	return enqueued, nil
}

// Maximum timeout for HTTP tasks.
// See https://cloud.google.com/tasks/docs/creating-http-target-tasks.
const maxCloudTasksTimeout = 30 * time.Minute

func (q *GCP) newTaskRequest(modulePath, version, suffix string) *taskspb.CreateTaskRequest {
	taskID := newTaskID(modulePath, version)
	relativeURI := fmt.Sprintf("/fetch/%s/@v/%s", modulePath, version)
	task := &taskspb.Task{
		Name:             fmt.Sprintf("%s/tasks/%s", q.queueName, taskID),
		DispatchDeadline: ptypes.DurationProto(maxCloudTasksTimeout),
	}
	if q.queueService != "" {
		task.MessageType = &taskspb.Task_AppEngineHttpRequest{
			AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
				HttpMethod:  taskspb.HttpMethod_POST,
				RelativeUri: relativeURI,
				AppEngineRouting: &taskspb.AppEngineRouting{
					Service: q.queueService,
				},
			},
		}
	} else {
		task.MessageType = &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod:          taskspb.HttpMethod_POST,
				Url:                 q.queueURL + relativeURI,
				AuthorizationHeader: q.token,
			},
		}
	}
	req := &taskspb.CreateTaskRequest{
		Parent: q.queueName,
		Task:   task,
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
func newTaskID(modulePath, version string) string {
	mv := modulePath + "@" + version
	// Compute a hash to use as a prefix, so the task IDs are distributed uniformly.
	// See https://cloud.google.com/tasks/docs/reference/rpc/google.cloud.tasks.v2#task
	// under "Task De-duplication".
	hasher := fnv.New32()
	io.WriteString(hasher, mv)
	hash := hasher.Sum32() % math.MaxUint16
	// Escape the name so it contains only valid characters. Do our best to make it readable.
	var b strings.Builder
	for _, r := range mv {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-':
			b.WriteRune(r)
		case r == '_':
			b.WriteString("__")
		case r == '/':
			b.WriteString("_-")
		case r == '@':
			b.WriteString("_v")
		case r == '.':
			b.WriteString("_o")
		default:
			fmt.Fprintf(&b, "_%04x", r)
		}
	}
	return fmt.Sprintf("%04x-%s", hash, &b)
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
func (q *InMemory) ScheduleFetch(ctx context.Context, modulePath, version, suffix string) (bool, error) {
	q.queue <- moduleVersion{modulePath, version}
	return true, nil
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
