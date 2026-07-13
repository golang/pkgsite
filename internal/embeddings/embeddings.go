// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// cloudPlatformScope is the OAuth2 scope required to call Google Cloud APIs.
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

	// outputDimensionality is the number of dimensions for output embeddings.
	// This must match the DB schema: embedding halfvec(256).
	outputDimensionality = 256

	// defaultTimeout is the HTTP request timeout for Vertex AI prediction.
	defaultTimeout = 10 * time.Second
)

// TaskType specifies the Vertex AI embedding task type.
// See https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/embeddings/task-types
type TaskType string

const (
	// TaskTypeDocument is used for indexing static document content.
	TaskTypeDocument TaskType = "RETRIEVAL_DOCUMENT"

	// TaskTypeQuery is used for user search queries.
	TaskTypeQuery TaskType = "RETRIEVAL_QUERY"
)

// Client is a client for the Vertex AI Embeddings API.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string

	projectID string
	location  string
	model     string
}

// NewClient creates a new Client.
// location is the GCP region (e.g., "us-central1").
// model is the embedding model name (e.g., "text-embedding-004").
func NewClient(ctx context.Context, location, model string) (*Client, error) {
	creds, err := google.FindDefaultCredentials(ctx, cloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("google.FindDefaultCredentials: %w", err)
	}

	projectID := creds.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		return nil, fmt.Errorf("GCP project ID is empty; set GOOGLE_CLOUD_PROJECT environment variable")
	}

	httpClient := oauth2.NewClient(ctx, creds.TokenSource)
	httpClient.Timeout = defaultTimeout

	return &Client{
		HTTPClient: httpClient,
		projectID:  projectID,
		location:   location,
		model:      model,
	}, nil
}

type predictRequest struct {
	Instances  []instance `json:"instances"`
	Parameters struct {
		OutputDimensionality int `json:"outputDimensionality,omitempty"`
	} `json:"parameters,omitempty"`
}

type instance struct {
	Content  string   `json:"content"`
	TaskType TaskType `json:"taskType,omitempty"`
}

type predictResponse struct {
	Predictions []struct {
		Embeddings struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	} `json:"predictions"`
}

// GenerateEmbeddings gets 256-dimensional embeddings for the given texts using the Vertex AI API.
func (c *Client) GenerateEmbeddings(ctx context.Context, texts []string, taskType TaskType) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	var reqBody predictRequest
	reqBody.Parameters.OutputDimensionality = outputDimensionality

	for _, text := range texts {
		reqBody.Instances = append(reqBody.Instances, instance{
			Content:  text,
			TaskType: taskType,
		})
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := c.BaseURL
	if url == "" {
		url = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
			c.location, c.projectID, c.location, c.model)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vertex AI predict failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var respBody predictResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, err
	}

	if len(respBody.Predictions) != len(texts) {
		return nil, fmt.Errorf("expected %d predictions, got %d", len(texts), len(respBody.Predictions))
	}

	results := make([][]float32, len(texts))
	for i, pred := range respBody.Predictions {
		results[i] = pred.Embeddings.Values
	}

	return results, nil
}
