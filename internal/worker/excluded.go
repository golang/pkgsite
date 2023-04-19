// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
)

type exclusion struct {
	prefix, reason string
}

// PopulateExcluded adds each element of excludedPrefixes to the excluded_prefixes
// table if it isn't already present.
func PopulateExcluded(ctx context.Context, cfg *config.Config, db *postgres.DB) error {
	location := cfg.DynamicExcludeLocation
	if location == "" {
		return nil
	}
	var r io.ReadCloser
	if strings.HasPrefix(location, "gs://") {
		log.Debugf(ctx, "reading exclusions config from %s", location)
		bucket, object, found := strings.Cut(location[5:], "/")
		if !found {
			return errors.New("bad GCS URL")
		}
		client, err := storage.NewClient(ctx)
		if err != nil {
			return err
		}
		defer client.Close()
		r, err = client.Bucket(bucket).Object(object).NewReader(ctx)
		if err != nil {
			return err
		}
	} else {
		var err error
		r, err = os.Open(location)
		if err != nil {
			return err
		}
	}
	defer r.Close()
	lines, err := readExcludedLines(ctx, r)
	if err != nil {
		return err
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "worker"
	}
	for _, line := range lines {
		present, err := db.IsExcluded(ctx, line.prefix)
		if err != nil {
			return fmt.Errorf("db.IsExcluded(%q): %v", line.prefix, err)
		}
		if !present {
			if err := db.InsertExcludedPrefix(ctx, line.prefix, user, line.reason); err != nil {
				return fmt.Errorf("db.InsertExcludedPrefix(%q, %q, %q): %v", line.prefix, user, line.reason, err)
			}
		}
	}
	return nil
}

func readExcludedLines(ctx context.Context, r io.Reader) ([]exclusion, error) {
	var lines []exclusion
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var prefix, reason string
		i := strings.IndexAny(line, " \t")
		if i >= 0 {
			prefix = line[:i]
			reason = strings.TrimSpace(line[i+1:])
		}
		if reason == "" {
			return nil, fmt.Errorf("missing reason in line %q", line)
		}
		lines = append(lines, exclusion{prefix, reason})
	}
	if s.Err() != nil {
		return nil, s.Err()
	}
	return lines, nil
}
