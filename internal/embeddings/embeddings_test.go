// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (f mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGenerateEmbeddings(t *testing.T) {
	mockResp := predictResponse{
		Predictions: []struct {
			Embeddings struct {
				Values []float32 `json:"values"`
			} `json:"embeddings"`
		}{
			{
				Embeddings: struct {
					Values []float32 `json:"values"`
				}{
					Values: []float32{0.1, 0.2, 0.3},
				},
			},
			{
				Embeddings: struct {
					Values []float32 `json:"values"`
				}{
					Values: []float32{0.4, 0.5, 0.6},
				},
			},
		},
	}
	respBytes, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("failed to marshal mock response: %v", err)
	}

	httpClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			expectedURL := "https://test-location-aiplatform.googleapis.com/v1/projects/test-project/locations/test-location/publishers/google/models/test-model:predict"
			if req.URL.String() != expectedURL {
				t.Errorf("expected URL %q, got %q", expectedURL, req.URL.String())
			}

			var reqBody predictRequest
			if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}

			if len(reqBody.Instances) != 2 {
				t.Errorf("expected 2 instances, got %d", len(reqBody.Instances))
			}
			if reqBody.Instances[0].Content != "hello" || reqBody.Instances[0].TaskType != TaskTypeDocument {
				t.Errorf("unexpected instance 0: %+v", reqBody.Instances[0])
			}
			if reqBody.Instances[1].Content != "world" || reqBody.Instances[1].TaskType != TaskTypeDocument {
				t.Errorf("unexpected instance 1: %+v", reqBody.Instances[1])
			}
			if reqBody.Parameters.OutputDimensionality != outputDimensionality {
				t.Errorf("expected OutputDimensionality %d, got %d", outputDimensionality, reqBody.Parameters.OutputDimensionality)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(respBytes)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	client := &Client{
		HTTPClient: httpClient,
		projectID:  "test-project",
		location:   "test-location",
		model:      "test-model",
	}

	got, err := client.GenerateEmbeddings(context.Background(), []string{"hello", "world"}, TaskTypeDocument)
	if err != nil {
		t.Fatalf("GenerateEmbeddings failed: %v", err)
	}

	want := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(got))
	}

	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("got[%d][%d] = %f, want %f", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestGenerateEmbeddingsError(t *testing.T) {
	httpClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("Internal error")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	client := &Client{
		HTTPClient: httpClient,
		projectID:  "test-project",
		location:   "test-location",
		model:      "test-model",
	}

	_, err := client.GenerateEmbeddings(context.Background(), []string{"hello"}, TaskTypeDocument)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vertex AI predict failed (status 500): Internal error") {
		t.Errorf("unexpected error message: %v", err)
	}
}
