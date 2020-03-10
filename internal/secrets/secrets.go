// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/storage"
	"golang.org/x/discovery/internal/derrors"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
)

var (
	gcsBucket = os.Getenv("GO_DISCOVERY_SECRETS_BUCKET")

	// kmsKeyName is the resource name of the cryptographic key used for encrypting and decrypting secrets.
	kmsKeyName = os.Getenv("GO_DISCOVERY_KMS_KEY_NAME")
)

// Get returns the named secret value as plaintext.
func Get(ctx context.Context, name string) (plaintext string, err error) {
	defer derrors.Add(&err, "secrets.Get(ctx, %q)", name)

	if gcsBucket == "" {
		return "", errors.New("missing environment variable GO_DISCOVERY_SECRETS_BUCKET")
	}
	if kmsKeyName == "" {
		return "", errors.New("missing environment variable GO_DISCOVERY_KMS_KEY_NAME")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("could not create GCS storage client: %v", err)
	}

	bkt := client.Bucket(gcsBucket)
	r, err := bkt.Object(name + ".encrypted").NewReader(ctx)
	if err != nil {
		return "", fmt.Errorf("could not create GCS object reader: %v", err)
	}
	defer func() {
		cerr := r.Close()
		if cerr != nil {
			err = fmt.Errorf("could not close GCS client: %v", cerr)
		}
	}()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("could not read from GCS object: %v", err)
	}

	d, err := decryptWithKMS(ctx, b)
	if err != nil {
		return "", fmt.Errorf("could not decrypt ciphertext using KMS: %v", err)
	}

	return string(d), nil
}

// Set writes the named secret value. It will overwrite an existing secret with
// the same name without returning an error.
func Set(ctx context.Context, name, plaintext string) (err error) {
	defer derrors.Add(&err, "secrets.Set(ctx, %q, plaintext)", name)

	if gcsBucket == "" {
		return errors.New("missing environment variable GO_DISCOVERY_SECRETS_BUCKET")
	}
	if kmsKeyName == "" {
		return errors.New("missing environment variable GO_DISCOVERY_KMS_KEY_NAME")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("could not create GCS storage client: %v", err)
	}

	bkt := client.Bucket(gcsBucket)
	w := bkt.Object(name + ".encrypted").NewWriter(ctx)
	defer func() {
		cerr := w.Close()
		if cerr != nil {
			err = fmt.Errorf("could not close GCS client: %v", cerr)
		}
	}()

	b, err := encryptWithKMS(ctx, []byte(plaintext))
	if err != nil {
		return fmt.Errorf("could not encrypt plaintext using KMS: %v", err)
	}

	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("could not write to object: %v", err)
	}

	return nil
}

func encryptWithKMS(ctx context.Context, plaintext []byte) ([]byte, error) {
	client, err := cloudkms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not create KMS client: %v", err)
	}

	req := &kmspb.EncryptRequest{
		Name:      kmsKeyName,
		Plaintext: plaintext,
	}

	resp, err := client.Encrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("could not encrypt using KMS: %v", err)
	}
	return resp.Ciphertext, nil
}

func decryptWithKMS(ctx context.Context, ciphertext []byte) ([]byte, error) {
	client, err := cloudkms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not create KMS client: %v", err)
	}

	req := &kmspb.DecryptRequest{
		Name:       kmsKeyName,
		Ciphertext: ciphertext,
	}

	resp, err := client.Decrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt using KMS: %v", err)
	}
	return resp.Plaintext, nil
}
