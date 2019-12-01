// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
)

func TestGetActiveExperiments(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	experiment := &internal.Experiment{Name: "test-experiment", Description: "test-description"}
	if err := testDB.updateExperiment(ctx, experiment); err != nil {
		t.Fatalf("unexpected error when updating non-existent experiment: %v", err)
	}
	got, err := testDB.GetActiveExperiments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d active experiments; want = 0", len(got))
	}
	if err := testDB.insertExperiment(ctx, experiment); err != nil {
		t.Fatalf("error inserting inactive experiment: %v", err)
	}
	got, err = testDB.GetActiveExperiments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d active experiments; want = 0", len(got))
	}

	experiment.Rollout = 50
	if err := testDB.updateExperiment(ctx, experiment); err != nil {
		t.Fatal(err)
	}
	got, err = testDB.GetActiveExperiments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %d active experiments; want = 1", len(got))
	}
	if diff := cmp.Diff(experiment, got[0]); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

}

func TestCannotInsertRolloutGreaterThan100(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	experiment := &internal.Experiment{
		Name:        "test-rollout-greater-than-one",
		Rollout:     101,
		Description: "test-description",
	}
	// Test cannot insert feature with rollout > 100.
	if err := testDB.insertExperiment(ctx, experiment); err == nil {
		t.Fatal(err)
	}

	experiment.Rollout = 100
	if err := testDB.insertExperiment(ctx, experiment); err != nil {
		t.Fatal(err)
	}
	// Test cannot update feature rollout to > 1.
	experiment.Rollout = 101
	if err := testDB.updateExperiment(ctx, experiment); err == nil {
		t.Fatal(err)
	}
}
