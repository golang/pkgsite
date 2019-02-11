// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sourcestorage

import (
	"context"
	"testing"

	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
)

func TestReadAndWrite(t *testing.T) {
	// memblob provides an in-memory blob implementation used for testing.
	b := &Bucket{memblob.OpenBucket(nil)}

	ctx := context.Background()

	key := "test-key"
	_, err := b.Read(ctx, key)
	if gcerrors.Code(err) != gcerrors.NotFound {
		t.Fatalf("b.Read(%q) error = %v, want %v", key, err, gcerrors.NotFound)
	}

	data := "hello world"
	if err = b.Write(ctx, key, []byte(data)); err != nil {
		t.Fatalf("b.Write(ctx, %q, %q) error: %v", key, data, err)
	}

	got, err := b.Read(ctx, key)
	if err != nil {
		t.Fatalf("b.Read(%q) error: %v", key, err)
	}
	if string(got) != data {
		t.Fatalf("b.Read(%s) = %s, want %s", key, got, data)
	}
}
