// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"strconv"

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
)

func recordEnqueue(ctx context.Context, status int) {
	stats.RecordWithTags(ctx,
		[]tag.Mutator{tag.Upsert(keyEnqueueStatus, strconv.Itoa(status))},
		enqueueStatus.M(int64(status)))
}
