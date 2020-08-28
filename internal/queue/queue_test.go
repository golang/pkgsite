// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
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
	for _, test := range []struct {
		name string
		cfg  config.Config
		want *taskspb.CreateTaskRequest
	}{
		{
			"AppEngine",
			config.Config{
				ProjectID:    "Project",
				LocationID:   "us-central1",
				QueueService: "Service",
			},
			&taskspb.CreateTaskRequest{
				Parent: "projects/Project/locations/us-central1/queues/queueID",
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
			},
		},
		{
			"non-AppEngine",
			config.Config{
				ProjectID:  "Project",
				LocationID: "us-central1",
				QueueURL:   "http://1.2.3.4:8000",
			},
			&taskspb.CreateTaskRequest{
				Parent: "projects/Project/locations/us-central1/queues/queueID",
				Task: &taskspb.Task{
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							HttpMethod: taskspb.HttpMethod_POST,
							Url:        "http://1.2.3.4:8000/fetch/mod/@v/v1.2.3",
						},
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gcp, err := newGCP(&test.cfg, nil, "queueID")
			if err != nil {
				t.Fatal(err)
			}
			got := gcp.newTaskRequest("mod", "v1.2.3", "suf", time.Minute)
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreFields(taskspb.Task{}, "Name")); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
