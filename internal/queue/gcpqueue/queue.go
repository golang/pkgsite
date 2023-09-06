// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gcpqueue provides a GCP queue implementation that can be used for
// asynchronous scheduling of fetch actions.
package gcpqueue

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
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/queue"
)

// New creates a new Queue with name queueName based on the configuration
// in cfg. When running locally, Queue uses numWorkers concurrent workers.
func New(ctx context.Context, cfg *config.Config, queueName string, numWorkers int, expGetter middleware.ExperimentGetter, processFunc queue.InMemoryProcessFunc) (queue.Queue, error) {
	if !serverconfig.OnGCP() {
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
		return queue.NewInMemory(ctx, numWorkers, names, processFunc), nil
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

// gcp provides a Queue implementation backed by the Google Cloud Tasks
// API.
type gcp struct {
	client    *cloudtasks.Client
	queueName string // full gcp name of the queue
	queueURL  string // non-AppEngine URL to post tasks to
	// token holds information that lets the task queue construct an authorized request to the worker.
	// Since the worker sits behind the IAP, the queue needs an identity token that includes the
	// identity of a service account that has access, and the client ID for the IAP.
	// We use the service account of the current process.
	token *taskspb.HttpRequest_OidcToken
}

// newGCP returns a new Queue that can be used to enqueue tasks using the
// cloud tasks API.  The given queueID should be the name of the queue in the
// cloud tasks console.
func newGCP(cfg *config.Config, client *cloudtasks.Client, queueID string) (_ *gcp, err error) {
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
	return &gcp{
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
func (q *gcp) ScheduleFetch(ctx context.Context, modulePath, version string, opts *queue.Options) (enqueued bool, err error) {
	defer derrors.WrapStack(&err, "queue.ScheduleFetch(%q, %q, %v)", modulePath, version, opts)
	if opts == nil {
		opts = &queue.Options{}
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

func (q *gcp) newTaskRequest(modulePath, version string, opts *queue.Options) *taskspb.CreateTaskRequest {
	taskID := newTaskID(modulePath, version)
	relativeURI := fmt.Sprintf("/fetch/%s/@v/%s", modulePath, version)
	var params []string
	if opts.Source != "" {
		params = append(params, fmt.Sprintf("%s=%s", queue.SourceParam, opts.Source))
	}
	if opts.DisableProxyFetch {
		params = append(params, fmt.Sprintf("%s=%s", queue.DisableProxyFetchParam, queue.DisableProxyFetchValue))
	}
	if len(params) > 0 {
		relativeURI += fmt.Sprintf("?%s", strings.Join(params, "&"))
	}

	task := &taskspb.Task{
		Name:             fmt.Sprintf("%s/tasks/%s", q.queueName, taskID),
		DispatchDeadline: durationpb.New(maxCloudTasksTimeout),
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

// Maximum timeout for HTTP tasks.
// See https://cloud.google.com/tasks/docs/creating-http-target-tasks.
const maxCloudTasksTimeout = 30 * time.Minute
