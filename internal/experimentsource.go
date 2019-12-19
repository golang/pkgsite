// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
)

// ExperimentSource is the interface used by the middleware to interact with
// experiments data.
type ExperimentSource interface {
	// GetActiveExperiments fetches active experiments from the
	// ExperimentSource.
	GetActiveExperiments(ctx context.Context) ([]*Experiment, error)
}
