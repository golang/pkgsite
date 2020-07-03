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
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func TestFrontendFetchForMasterVersion(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	experiments := []string{
		internal.ExperimentFrontendFetch,
		internal.ExperimentMasterVersion,
		internal.ExperimentInsertDirectories,
		internal.ExperimentUsePathInfo,
	}
	ctx := experiment.NewContext(context.Background(), experiments...)
	for _, e := range experiments {
		if err := testDB.InsertExperiment(ctx, &internal.Experiment{
			Name:        e,
			Description: e,
			Rollout:     100,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Add sample.ModulePath@sample.VersionString to the database.
	// Check that GET /sample.ModulePath returns a 200.
	testModule := &proxy.TestModule{
		ModulePath: sample.ModulePath,
		Version:    "v1.0.0",
		Files: map[string]string{
			"found.go":       "package found\nconst Value = 123",
			"dir/pkg/pkg.go": "package pkg\nconst Value = 321",
			"LICENSE":        testhelper.MITLicense,
		},
	}
	q, teardown := setupQueue(ctx, t, []*proxy.TestModule{testModule}, experiments...)
	defer teardown()
	ts := setupFrontend(ctx, t, q)

	for _, req := range []struct {
		urlPath string
		status  int
	}{
		// Validate that the sample.ModulePath does not exist in the database.
		{sample.ModulePath, http.StatusNotFound},
		// Insert the latest version of the module using the frontend fetch
		// endpoint.
		{fmt.Sprintf("fetch/%s", sample.ModulePath), http.StatusOK},
		// Validate that sample.ModulePath@master does not exist in the
		// database. GET /sample.ModulePath@master should return a 404.
		{fmt.Sprintf("%s@master", sample.ModulePath), http.StatusNotFound},
		// Insert the master version of the module using the frontend fetch
		// endpoint.
		{fmt.Sprintf("fetch/%s@master", sample.ModulePath), http.StatusOK},
		// Check that GET /sample.ModulePath@master now returns a 200.
		{fmt.Sprintf("%s@master", sample.ModulePath), http.StatusOK},
		// Check that GET /mod/sample.ModulePath@master also returns a 200.
		{fmt.Sprintf("mod/%s@master", sample.ModulePath), http.StatusOK},
	} {
		validateResponse(t, ts.URL+"/"+req.urlPath, req.status, nil)
	}
}
