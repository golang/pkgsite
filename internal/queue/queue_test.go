// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal/config"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
)

func TestNewTaskID(t *testing.T) {
	// Verify that the task ID is the same within taskIDChangeInterval and changes
	// afterwards.
	var (
		module               = "mod"
		version              = "ver"
		taskIDChangeInterval = 3 * time.Hour
	)
	tm := time.Now().Truncate(taskIDChangeInterval)
	id1 := newTaskID(module, version, tm, taskIDChangeInterval)
	id2 := newTaskID(module, version, tm.Add(taskIDChangeInterval/2), taskIDChangeInterval)
	if id1 != id2 {
		t.Error("wanted same task ID, got different")
	}
	id3 := newTaskID(module, version, tm.Add(taskIDChangeInterval+1), taskIDChangeInterval)
	if id1 == id3 {
		t.Error("wanted different task ID, got same")
	}
}

func TestNewTaskRequest(t *testing.T) {
	for vari, val := range map[string]string{
		"GOOGLE_CLOUD_PROJECT": "Project",
		"GAE_SERVICE":          "Service",
	} {
		vari := vari
		prev := os.Getenv(vari)
		os.Setenv(vari, val)
		defer func() { os.Setenv(vari, prev) }()
	}

	cfg, err := config.Init(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const queueID = "queueID"
	gcp := newGCP(cfg, nil, queueID)
	got := gcp.newTaskRequest("mod", "v1.2.3", "suf", time.Minute)
	want := &taskspb.CreateTaskRequest{
		Parent: "projects/Project/locations/us-central1/queues/" + queueID,
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod:  taskspb.HttpMethod_POST,
					RelativeUri: "/fetch/mod/@v/v1.2.3",
					AppEngineRouting: &taskspb.AppEngineRouting{
						Service: "Service",
					},
				},
			},
		},
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(taskspb.Task{}, "Name")); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
