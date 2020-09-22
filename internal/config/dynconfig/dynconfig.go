// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dynconfig supports dynamic configuration for pkgsite services.
// Dynamic configuration is read from a file and can change over the lifetime of
// the process.
package dynconfig

import (
	"context"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"github.com/ghodss/yaml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// DynamicConfig holds configuration that can change over the lifetime of the
// process. It is loaded from a GCS file or other external source.
type DynamicConfig struct {
	// Fields can be added at any time, but removing or changing a field
	// requires careful coordination with the config file contents.

	Experiments []*internal.Experiment
}

// Read reads dynamic configuration from the given GCS bucket and object.
func Read(ctx context.Context, bucket, object string) (_ *DynamicConfig, err error) {
	defer derrors.Wrap(&err, "dynconfig.Read(%q, %q)", bucket, object)

	log.Infof(ctx, "reading dynamic config from gs://%s/%s", bucket, object)
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	r, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses yamlData as a YAML description of DynamicConfig.
func Parse(yamlData []byte) (_ *DynamicConfig, err error) {
	defer derrors.Wrap(&err, "dynconfig.Parse(data)")

	var dc DynamicConfig
	if err := yaml.Unmarshal(yamlData, &dc); err != nil {
		return nil, err
	}
	return &dc, nil
}
