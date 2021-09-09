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
	"golang.org/x/pkgsite/internal"
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
	ScheduleFetch(ctx context.Context, modulePath, version string, opts *Options) (bool, error)
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
	log.Infof(ctx, "enqueuing at %s with queueURL=%q", g.queueName, g.queueURL)
	return g, nil
}

// GCP provides a Queue implementation backed by the Google Cloud Tasks
// API.
type GCP struct {
	client    *cloudtasks.Client
	queueName string // full GCP name of the queue
	queueURL  string // non-AppEngine URL to post tasks to
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
	if cfg.QueueURL == "" {
		return nil, errors.New("empty QueueURL")
	}
	if cfg.ServiceAccount == "" {
		return nil, errors.New("empty ServiceAccount")
	}
	if cfg.QueueAudience == "" {
		return nil, errors.New("empty QueueAudience")
	}
	return &GCP{
		client:    client,
		queueName: fmt.Sprintf("projects/%s/locations/%s/queues/%s", cfg.ProjectID, cfg.LocationID, queueID),
		queueURL:  cfg.QueueURL,
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
func (q *GCP) ScheduleFetch(ctx context.Context, modulePath, version string, opts *Options) (enqueued bool, err error) {
	defer derrors.WrapStack(&err, "queue.ScheduleFetch(%q, %q, %v)", modulePath, version, opts)
	if opts == nil {
		opts = &Options{}
	}
	// Cloud Tasks enforces an RPC timeout of at most 30s. I couldn't find this
	// in the documentation, but using a larger value, or no timeout, results in
	// an InvalidArgument error with the text "The deadline cannot be more than
	// 30s in the future."
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if modulePath == internal.UnknownModulePath {
		return false, errors.New("given unknown module path")
	}
	req := q.newTaskRequest(modulePath, version, opts)
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

// Maximum timeout for HTTP tasks.
// See https://cloud.google.com/tasks/docs/creating-http-target-tasks.
const maxCloudTasksTimeout = 30 * time.Minute

const (
	DisableProxyFetchParam = "proxyfetch"
	DisableProxyFetchValue = "off"
	SourceParam            = "source"
	SourceFrontendValue    = "frontend"
)

func (q *GCP) newTaskRequest(modulePath, version string, opts *Options) *taskspb.CreateTaskRequest {
	taskID := newTaskID(modulePath, version)
	relativeURI := fmt.Sprintf("/fetch/%s/@v/%s", modulePath, version)
	var params []string
	if opts.Source != "" {
		params = append(params, fmt.Sprintf("%s=%s", SourceParam, opts.Source))
	}
	if opts.DisableProxyFetch {
		params = append(params, fmt.Sprintf("%s=%s", DisableProxyFetchParam, DisableProxyFetchValue))
	}
	if len(params) > 0 {
		relativeURI += fmt.Sprintf("?%s", strings.Join(params, "&"))
	}

	task := &taskspb.Task{
		Name:             fmt.Sprintf("%s/tasks/%s", q.queueName, taskID),
		DispatchDeadline: ptypes.DurationProto(maxCloudTasksTimeout),
	}
	task.MessageType = &taskspb.Task_HttpRequest{
		HttpRequest: &taskspb.HttpRequest{
			HttpMethod:          taskspb.HttpMethod_POST,
			Url:                 q.queueURL + relativeURI,
			AuthorizationHeader: q.token,
		},
	}
	req := &taskspb.CreateTaskRequest{
		Parent: q.queueName,
		Task:   task,
	}
	// If suffix is non-empty, append it to the task name. This lets us force reprocessing
	// of tasks that would normally be de-duplicated.
	if opts.Suffix != "" {
		req.Task.Name += "-" + opts.Suffix
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

type inMemoryProcessFunc func(context.Context, string, string) (int, error)

// NewInMemory creates a new InMemory that asynchronously fetches
// from proxyClient and stores in db. It uses workerCount parallelism to
// execute these fetches.
func NewInMemory(ctx context.Context, workerCount int, experiments []string, processFunc inMemoryProcessFunc) *InMemory {
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
