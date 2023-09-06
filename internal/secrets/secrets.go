// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package secrets is used to interact with secretmanager.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"golang.org/x/pkgsite/internal/derrors"
)

// Get returns the named secret value as plaintext.
func Get(ctx context.Context, name string) (plaintext string, err error) {
	defer derrors.Add(&err, "secrets.Get(ctx, %q)", name)

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return "", errors.New("need GOOGLE_CLOUD_PROJECT environment variable")
	}
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	result, err := client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, name),
	})
	if err != nil {
		return "", err
	}
	return string(result.Payload.Data), nil
}
