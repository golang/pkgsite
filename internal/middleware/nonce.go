// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package middleware implements a simple middleware pattern for http handlers,
// along with implementations for some common middlewares.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	nonceCtxKey = "CSPNonce"
	nonceLen    = 20
)

// GetNonce returns whether a nonce relative to ctx and the nonce if available.
func GetNonce(ctx context.Context) (nonce string, ok bool) {
	if v := ctx.Value(nonceCtxKey); v != nil {
		nonce, ok = v.(string)
	}
	return nonce, ok
}

// setNonce updates the given context with nonce information if not available.
func setNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, nonceCtxKey, nonce)
}

func generateNonce() (string, error) {
	nonceBytes := make([]byte, nonceLen)
	if _, err := io.ReadAtLeast(rand.Reader, nonceBytes, nonceLen); err != nil {
		return "", fmt.Errorf("io.ReadAtLeast(rand.Reader, nonceBytes, %d): %v", nonceLen, err)
	}
	return base64.StdEncoding.EncodeToString(nonceBytes), nil
}
