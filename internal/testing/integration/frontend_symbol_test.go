// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"golang.org/x/pkgsite/internal/frontend/versions"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/symbol"
)

func TestSymbols(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx := context.Background()
	processVersions(ctx, t, testModules)

	for _, test := range []struct {
		name, pkgPath, modulePath string
	}{
		{
			"test v1 module (rsc.io quote)",
			"rsc.io/quote",
			"rsc.io/quote",
		},
		{
			"test v3 module (rsc.io quote v3)",
			"rsc.io/quote/v3",
			"rsc.io/quote/v3",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Get api data.
			files, err := symbol.LoadAPIFiles(test.pkgPath, "../../symbol/testdata")
			if err != nil {
				t.Fatal(err)
			}
			apiVersions, err := symbol.ParsePackageAPIInfo(files)
			if err != nil {
				t.Fatal(err)
			}

			// Get frontend data.
			urlPath := fmt.Sprintf("/%s?tab=versions&content=json", test.pkgPath)
			body := getFrontendPage(t, urlPath)
			var vd versions.VersionsDetails
			if err := json.Unmarshal([]byte(body), &vd); err != nil {
				t.Fatalf("json.Unmarshal: %v\n %s", err, body)
			}
			sh, err := versions.ParseVersionsDetails(&vd)
			if err != nil {
				t.Fatal(err)
			}

			// Compare the output of these two data sources.
			errs, err := symbol.CompareAPIVersions(test.pkgPath,
				apiVersions[test.pkgPath], sh)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range errs {
				t.Error(e)
			}
		})
	}
}
