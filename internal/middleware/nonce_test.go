// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package middleware implements a simple middleware pattern for http handlers,
// along with implementations for some common middlewares.
package middleware

import (
	"context"
	"testing"
)

func TestGetAndSetNonce(t *testing.T) {
	ctx := context.Background()
	if nonce, ok := GetNonce(ctx); ok {
		t.Fatalf("getNonce(ctx) = %q, %t; expected the empty string", nonce, ok)
	}

	nonce, err := generateNonce()
	if err != nil {
		t.Fatalf("generateNonce(): %v", err)
	}
	if len(nonce) < nonceLen {
		t.Fatalf("GetNonce(ctx) = %q; want nonceLen > %d; got nonceLen = %d", nonce, nonceLen, len(nonce))
	}

	ctx = setNonce(ctx, nonce)
	gotNonce, ok := GetNonce(ctx)
	if !ok {
		t.Fatalf("GetNonce(ctx) = %q, %t; expected nonce to be returned", gotNonce, ok)
	}
	if nonce != gotNonce {
		t.Fatalf("GetNonce(ctx) = %q; want = %q", gotNonce, nonce)
	}
}
