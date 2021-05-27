// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"strconv"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// keyEnqueueStatus is a census tag used to keep track of the status
	// of the modules being enqueued.
	keyEnqueueStatus = tag.MustNewKey("enqueue.status")
	enqueueStatus    = stats.Int64(
		"go-discovery/worker_enqueue_count",
		"The status of a module version enqueued to Cloud Tasks.",
		stats.UnitDimensionless,
	)
	// EnqueueResponseCount counts worker enqueue responses by response type.
	EnqueueResponseCount = &view.View{
		Name:        "go-discovery/worker-enqueue/count",
		Measure:     enqueueStatus,
		Aggregation: view.Count(),
		Description: "Worker enqueue request count",
		TagKeys:     []tag.Key{keyEnqueueStatus},
	}

	processingLag = stats.Int64(
		"go-discovery/worker_processing_lag",
		"Time from appearing in the index to being processed.",
		stats.UnitSeconds,
	)
	ProcessingLag = &view.View{
		Name:        "go-discovery/worker_processing_lag",
		Measure:     processingLag,
		Aggregation: view.LastValue(),
		Description: "worker processing lag",
	}

	unprocessedModules = stats.Int64(
		"go-discovery/unprocessed_modules_count",
		"Number of unprocessed modules (status = 0 or >= 500).",
		stats.UnitDimensionless,
	)

	UnprocessedModules = &view.View{
		Name:        "go-discovery/unprocessed_modules/count",
		Measure:     unprocessedModules,
		Aggregation: view.LastValue(),
		Description: "number of unprocessed modules",
	}
)

func recordEnqueue(ctx context.Context, status int) {
	stats.RecordWithTags(ctx,
		[]tag.Mutator{tag.Upsert(keyEnqueueStatus, strconv.Itoa(status))},
		enqueueStatus.M(int64(status)))
}

func recordProcessingLag(ctx context.Context, d time.Duration) {
	stats.Record(ctx, processingLag.M(d.Milliseconds()/1000))
}

func recordUnprocessedModules(ctx context.Context, n int) {
	stats.Record(ctx, unprocessedModules.M(int64(n)))
}
