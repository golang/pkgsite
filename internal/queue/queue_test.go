// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/config"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/protobuf/proto"
)

func TestNewTaskID(t *testing.T) {
	for _, test := range []struct {
		modulePath, version string
		want                string
	}{
		{"m-1", "v2", "acc5-m-1_vv2"},
		{"my_module", "v1.2.3", "0cb9-my__module_vv1_o2_o3"},
		{"µπΩ/github.com", "v2.3.4-ß", "a49c-_00b5_03c0_03a9_-github_ocom_vv2_o3_o4-_00df"},
	} {
		got := newTaskID(test.modulePath, test.version)
		if got != test.want {
			t.Errorf("%s@%s: got %s, want %s", test.modulePath, test.version, got, test.want)
		}
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
					DispatchDeadline: ptypes.DurationProto(maxCloudTasksTimeout),
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
				ProjectID:      "Project",
				LocationID:     "us-central1",
				QueueURL:       "http://1.2.3.4:8000",
				ServiceAccount: "sa",
				QueueAudience:  "qa",
			},
			&taskspb.CreateTaskRequest{
				Parent: "projects/Project/locations/us-central1/queues/queueID",
				Task: &taskspb.Task{
					DispatchDeadline: ptypes.DurationProto(maxCloudTasksTimeout),
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							HttpMethod: taskspb.HttpMethod_POST,
							Url:        "http://1.2.3.4:8000/fetch/mod/@v/v1.2.3",
							AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
								OidcToken: &taskspb.OidcToken{
									ServiceAccountEmail: "sa",
									Audience:            "qa",
								},
							},
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
			got := gcp.newTaskRequest("mod", "v1.2.3", "suf")
			test.want.Task.Name = got.Task.Name
			if diff := cmp.Diff(test.want, got, cmp.Comparer(proto.Equal)); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
