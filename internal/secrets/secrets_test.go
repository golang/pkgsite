// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"testing"

	"cloud.google.com/go/storage"
)

var useCloud = flag.Bool("use_cloud", false, "Whether to use Google Cloud services in tests")

func TestGetSet(t *testing.T) {
	if !*useCloud {
		return
	}

	gcsBucket = "go-discovery-secrets-test"
	kmsKeyName = "projects/go-discovery/locations/global/keyRings/testing-key-ring/cryptoKeys/key_for_testing"

	name, val := "my_credential_"+randomStr(t), "🤭"
	ctx := context.Background()

	defer func() {
		client, err := storage.NewClient(ctx)
		if err != nil {
			t.Fatalf("storage.NewClient returned unexpected error: %v", err)
		}

		bkt := client.Bucket(gcsBucket)
		if err := bkt.Object(name + ".encrypted").Delete(ctx); err != nil {
			t.Errorf("(*Object).Delete returned unexpected error: %v", err)
		}
	}()

	if err := Set(ctx, name, val); err != nil {
		t.Fatalf("Set returned unexpected error: %v", err)
	}
	result, err := Get(ctx, name)
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if got, want := result, val; got != want {
		t.Errorf("Get: got %v; want %v", got, want)
	}
}

func randomStr(t *testing.T) string {
	buf := make([]byte, 10)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(buf)
}
