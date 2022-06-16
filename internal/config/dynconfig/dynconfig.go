// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dynconfig supports dynamic configuration for pkgsite services.
// Dynamic configuration is read from a file and can change over the lifetime of
// the process.
package dynconfig

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

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

// Read reads dynamic configuration from the given location.
// Location may be of the form gs://bucket/object, denoting a GCS bucket.
// Otherwise it is interpreted as a filename.
func Read(ctx context.Context, location string) (_ *DynamicConfig, err error) {
	defer derrors.Wrap(&err, "dynconfig.Read(%q)", location)

	log.Debugf(ctx, "reading dynamic config from %s", location)
	var r io.ReadCloser
	if strings.HasPrefix(location, "gs://") {
		bucket, object, found := strings.Cut(location[5:], "/")
		if !found {
			return nil, errors.New("bad GCS URL")
		}
		if err != nil {
			return nil, err
		}
		client, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		defer client.Close()
		r, err = client.Bucket(bucket).Object(object).NewReader(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		r, err = os.Open(location)
		if err != nil {
			return nil, err
		}
	}
	defer r.Close()
	data, err := io.ReadAll(r)
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
