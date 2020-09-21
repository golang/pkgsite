// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"io/ioutil"
	"log"

	"cloud.google.com/go/storage"
	"github.com/ghodss/yaml"
	"golang.org/x/pkgsite/internal/derrors"
)

// DynamicConfig holds configuration that can change over the lifetime of the
// process. It is loaded from a GCS file or other external source.
type DynamicConfig struct {
	// Fields can be added at any time, but removing or changing a field
	// requires careful coordination with the config file contents.

	Experiments []Experiment
}

// An experiment, as dynamically configured.
// Intended to match internal.Experiment, but we can't use
// that directly due to an import cycle.
type Experiment struct {
	Name        string
	Rollout     uint
	Description string
}

// ReadDynamic reads the dynamic configuration.
func (c *Config) ReadDynamic(ctx context.Context) (*DynamicConfig, error) {
	return ReadDynamic(ctx, c.bucket, c.dynamicObject)
}

// ReadDynamic reads dynamic configuration from the given GCS bucket and object.
func ReadDynamic(ctx context.Context, bucket, object string) (_ *DynamicConfig, err error) {
	defer derrors.Wrap(&err, "ReadDynamic(%q, %q)", bucket, object)

	log.Printf("reading experiments from gs://%s/%s", bucket, object)
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
	return ParseDynamic(data)
}

// ParseDynamic parses yamlData as a YAML description of DynamicConfig.
func ParseDynamic(yamlData []byte) (_ *DynamicConfig, err error) {
	defer derrors.Wrap(&err, "ParseDynamic(data)")

	var dc DynamicConfig
	if err := yaml.Unmarshal(yamlData, &dc); err != nil {
		return nil, err
	}
	return &dc, nil
}
