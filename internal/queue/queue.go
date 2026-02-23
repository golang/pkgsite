// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides a queue interface that can be used for
// asynchronous scheduling of fetch actions.
package queue

import (
	"context"
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
