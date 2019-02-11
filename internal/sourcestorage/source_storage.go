// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sourcestorage

import (
	"context"
	"fmt"

	"gocloud.dev/blob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
)

type Bucket struct {
	*blob.Bucket
}

// OpenBucket connects to the given bucket, or creates a new one if it does not
// exists.
func OpenBucket(ctx context.Context, bucket string) (*Bucket, error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("sourcestorage.OpenBucket gcp.DefaultCredentials error: %v", err)
	}

	// Create an HTTP client.
	client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, fmt.Errorf("sourcestorage.OpenBucket gcp.NewHTTPClient error: %v", err)
	}

	// Create a *blob.Bucket.
	b, err := gcsblob.OpenBucket(ctx, client, bucket, nil)
	if err != nil {
		return nil, fmt.Errorf("sourcestorage.OpenBucket gcsblob.OpenBucket error: %v", err)
	}
	return &Bucket{b}, nil
}

// Write writes data to the bucket at the specified key. If data already exists
// at this key, it will be replaced.
func (b *Bucket) Write(ctx context.Context, key string, data []byte) error {
	return b.WriteAll(ctx, key, data, nil)
}

// Read gets data from the bucket at specified key.
func (b *Bucket) Read(ctx context.Context, key string) ([]byte, error) {
	return b.ReadAll(ctx, key)
}
