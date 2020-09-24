// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secrets

import (
	"context"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("no GOOGLE_CLOUD_PROJECT environment variable")
	}
	// "test-secret" is the only secret which can be read with default permissions.
	got, err := Get(context.Background(), "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	const want = "xyzzy"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
