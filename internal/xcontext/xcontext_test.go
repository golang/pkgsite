// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xcontext

import (
	"context"
	"testing"
	"time"
)

type ctxKey string

var key = ctxKey("key")

func TestDetach(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ctx = context.WithValue(ctx, key, "value")
	dctx := Detach(ctx)
	// Detached context has the same values.
	got, ok := dctx.Value(key).(string)
	if !ok || got != "value" {
		t.Errorf("Value: got (%v, %t), want 'value', true", got, ok)
	}
	// Detached context doesn't time out.
	time.Sleep(500 * time.Millisecond)
	if err := ctx.Err(); err != context.DeadlineExceeded {
		t.Fatalf("original context Err: got %v, want DeadlineExceeded", err)
	}
	if err := dctx.Err(); err != nil {
		t.Errorf("detached context Err: got %v, want nil", err)
	}
}
