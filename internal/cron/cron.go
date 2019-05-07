// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
)

const fetchTimeout = 5 * time.Minute

// FetchAndStoreVersions queries indexURL for new versions and writes them to
// the version_logs table.
func fetchAndStoreVersions(ctx context.Context, idxClient *index.Client, db *postgres.DB) ([]*internal.VersionLog, error) {
	since, err := db.LatestProxyIndexUpdate(ctx)
	if err != nil {
		return nil, fmt.Errorf("db.LatestProxyIndexUpdate(): %v", err)
	}

	logs, err := idxClient.GetVersions(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("idxClient.GetVersions(ctx, %v): %v", since, err)
	}

	if err = db.InsertVersionLogs(ctx, logs); err != nil {
		return nil, fmt.Errorf("db.InsertVersionLogs(ctx, %v): %v", logs, err)
	}
	return logs, nil
}

// fetchIndexVersions makes a request to the fetch service for each index
// version. It uses workerCount number of goroutines to make these requests.
//
// Responses for each request are sent via the given responses channel, which
// is closed after all responses have been sent. This is a temporary measure in
// order to support the deprecated fetchVersions function.  In the future the
// 'fetchIndexVersions' functionality will be moved into the /fetchversions/
// handler.
func fetchIndexVersions(ctx context.Context, client *fetch.Client, requests []*fetch.Request, workerCount int, responses chan<- *fetch.Response) error {
	// Use a buffered channel as a semaphore for controlling access to a
	// goroutine.
	sem := make(chan struct{}, workerCount)

	defer func() {
		// Make sure all the workers are done before closing the channel.
		// The semaphore should be collected automatically once it is closed.
		for i := 0; i < workerCount; i++ {
			sem <- struct{}{}
		}
		close(responses)
	}()

	// For each item in logs, block until either ctx is done or
	// another worker is available. Break when ctx.Done() has closed.
	for i, request := range requests {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		// If a worker is available, make a request to the fetch service inside a
		// goroutine and wait for it to finish.
		go func(i int, request *fetch.Request) {
			defer func() { <-sem }()

			log.Printf("Fetch requested: %q %q (workerCount = %d)", request.ModulePath, request.Version, workerCount)

			fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
			responses <- client.FetchVersion(fetchCtx, request)
		}(i, request)
	}
	return nil
}

// fetchVersions makes a request to the fetch service for each entry in logs.
// It uses workerCount number of goroutines to make these requests.
//
// Deprecated: use fetchIndexVersions instead.
func fetchVersions(ctx context.Context, client *fetch.Client, logs []*internal.VersionLog, workerCount int) error {
	var requests []*fetch.Request
	for _, log := range logs {
		requests = append(requests, &fetch.Request{
			ModulePath: log.ModulePath,
			Version:    log.Version,
		})
	}
	var (
		responses = make(chan *fetch.Response, 10)
		err       error
	)
	logDone := make(chan struct{})
	go func() {
		for resp := range responses {
			// response will be 200 or 201, typically
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				log.Printf("got non-success response %v", resp)
			}
		}
		logDone <- struct{}{}
	}()
	err = fetchIndexVersions(ctx, client, requests, workerCount, responses)
	<-logDone
	return err
}
