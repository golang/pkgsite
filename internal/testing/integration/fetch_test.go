// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres"
)

func TestFrontendFetchForMasterVersion(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	// Add a module to the database.
	// Check that GET of the module's path returns a 200.
	ctx := context.Background()
	ctx = experiment.NewContext(ctx, internal.ExperimentDoNotInsertNewDocumentation)
	q, teardown := setupQueue(ctx, t, testModules[:1], internal.ExperimentDoNotInsertNewDocumentation)
	const modulePath = "example.com/basic"
	defer teardown()
	ts := setupFrontend(ctx, t, q, nil)

	for _, req := range []struct {
		method, urlPath string
		status          int
	}{
		// Validate that the modulePath does not exist in the database.
		{http.MethodGet, modulePath, http.StatusNotFound},
		// Insert the latest version of the module using the frontend fetch
		// endpoint.
		{http.MethodPost, fmt.Sprintf("fetch/%s", modulePath), http.StatusOK},
		// Validate that modulePath@master does not exist in the
		// database. GET /modulePath@master should return a 404.
		{http.MethodGet, fmt.Sprintf("%s@master", modulePath), http.StatusNotFound},
		// Insert the master version of the module using the frontend fetch
		// endpoint.
		{http.MethodPost, fmt.Sprintf("fetch/%s@master", modulePath), http.StatusOK},
		// Check that GET /modulePath@master now returns a 200.
		{http.MethodGet, fmt.Sprintf("%s@master", modulePath), http.StatusOK},
		// Check that GET /mod/modulePath@master also returns a 200.
		{http.MethodGet, fmt.Sprintf("mod/%s@master", modulePath), http.StatusOK},
	} {
		testURL := ts.URL + "/" + req.urlPath
		validateResponse(t, req.method, testURL, req.status, nil)
	}
}
