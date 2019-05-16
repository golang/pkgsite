// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

// fetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_state_table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func fetchAndUpdateState(ctx context.Context, modulePath, version string, client *proxy.Client, db *postgres.DB) (int, error) {
	var (
		code     = http.StatusOK
		fetchErr error
	)
	if fetchErr = fetch.FetchAndInsertVersion(modulePath, version, client, db); fetchErr != nil {
		log.Printf("Error executing fetch: %v", fetchErr)
		if derrors.IsNotFound(fetchErr) {
			code = http.StatusNotFound
		} else {
			code = http.StatusInternalServerError
		}
	}

	if err := db.UpsertVersionState(ctx, modulePath, version, time.Time{}, code, fetchErr); err != nil {
		log.Printf("db.UpsertVersionState(ctx, %q, %q, %q, %v): %q", modulePath, version, code, fetchErr, err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}

	return code, fetchErr
}
