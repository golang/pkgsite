// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package middleware implements a simple middleware pattern for http handlers,
// along with implementations for some common middlewares.
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	nonceCtxKey = "CSPNonce"
	nonceLen    = 20
)

func generateNonce() (string, error) {
	nonceBytes := make([]byte, nonceLen)
	if _, err := io.ReadAtLeast(rand.Reader, nonceBytes, nonceLen); err != nil {
		return "", fmt.Errorf("io.ReadAtLeast(rand.Reader, nonceBytes, %d): %v", nonceLen, err)
	}
	return base64.StdEncoding.EncodeToString(nonceBytes), nil
}
