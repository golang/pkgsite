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
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/net/context/ctxhttp"
)

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
