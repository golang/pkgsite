// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/net/context/ctxhttp"
)

const fetchTimeout = 5 * time.Minute

// FetchAndStoreVersions queries indexURL for new versions and writes them to
// the version_logs table.
func FetchAndStoreVersions(ctx context.Context, indexURL string, db *postgres.DB) ([]*internal.VersionLog, error) {
	t, err := db.LatestProxyIndexUpdate(ctx)
	if err != nil {
		return nil, fmt.Errorf("db.LatestProxyIndexUpdate(): %v", err)
	}

	logs, err := getVersionsFromIndex(ctx, indexURL, t)
	if err != nil {
		return nil, fmt.Errorf("getVersionsFromIndex(ctx, %q, %v): %v", indexURL, t, err)
	}

	if err = db.InsertVersionLogs(ctx, logs); err != nil {
		return nil, fmt.Errorf("db.InsertVersionLogs(ctx, %v): %v", logs, err)
	}
	return logs, nil
}

// getVersionsFromIndex makes a request to indexURL/<since> and returns the
// the response as a []*internal.VersionLog.
func getVersionsFromIndex(ctx context.Context, indexURL string, since time.Time) ([]*internal.VersionLog, error) {
	u := fmt.Sprintf("%s?since=%s", strings.TrimRight(indexURL, "/"), since.Format(time.RFC3339))
	r, err := ctxhttp.Get(ctx, nil, u)
	if err != nil {
		return nil, fmt.Errorf("ctxhttp.Get(ctx, nil, %q): %v", u, err)
	}
	defer r.Body.Close()

	var logs []*internal.VersionLog
	dec := json.NewDecoder(r.Body)

	// The module index returns a stream of JSON objects formatted with newline
	// as the delimiter. For each version log, we want to set source to
	// "proxy-index" and created_at to the time right before the proxy index is
	// queried.
	for dec.More() {
		var l internal.VersionLog
		if err := dec.Decode(&l); err != nil {
			log.Printf("dec.Decode: %v", err)
			continue
		}
		l.Source = internal.VersionSourceProxyIndex
		// The created_at column is without timestamp, so we must normalize to UTC.
		l.CreatedAt = l.CreatedAt.UTC()
		logs = append(logs, &l)
	}
	return logs, nil
}

// FetchVersions makes a request to the fetch service for each entry in logs.
// It uses workerCount number of goroutines to make these requests.
func FetchVersions(ctx context.Context, client *fetch.Client, logs []*internal.VersionLog, workerCount int) error {
	// Use a buffered channel as a semaphore for controlling access to a
	// goroutine.
	sem := make(chan struct{}, workerCount)

	defer func() {
		// Make sure all the workers are done before closing the channel.
		// The semaphore should be collected automatically once it is closed.
		for i := 0; i < workerCount; i++ {
			sem <- struct{}{}
		}
	}()

	// For each item in logs, block until either ctx is done or
	// another worker is available. Break when ctx.Done() has closed.
	for i, input := range logs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		// If a worker is available, make a request to the fetch service inside a
		// goroutine and wait for it to finish.
		go func(i int, input *internal.VersionLog) {
			defer func() { <-sem }()

			log.Printf("Fetch requested: %q %q (workerCount = %d)", input.ModulePath, input.Version, workerCount)

			fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
			if err := client.FetchVersion(fetchCtx, input.ModulePath, input.Version); err != nil {
				log.Printf("client.FetchVersion(fetchCtx, %q, %q): %v", input.ModulePath, input.Version, err)
			}
		}(i, input)
	}
	return nil
}
