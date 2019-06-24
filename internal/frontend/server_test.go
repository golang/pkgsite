// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/discovery/internal/postgres"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

func TestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	if err := testDB.InsertVersion(ctx, postgres.SampleVersion(), postgres.SampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%+v): %v", postgres.SampleVersion(), err)
	}
	if err := testDB.InsertDocuments(ctx, postgres.SampleVersion()); err != nil {
		t.Fatalf("testDB.InsertDocument(%+v): %v", postgres.SampleVersion(), err)
	}
	testDB.RefreshSearchDocuments(ctx)

	for _, urlPath := range []string{
		"/static/",
		"/license-policy/",
		"/favicon.ico",
		fmt.Sprintf("/search/?q=%s", postgres.SamplePackage.Name),
		fmt.Sprintf("/pkg/%s", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s@%s", postgres.SamplePackage.Path, postgres.SampleVersion().Version),
		fmt.Sprintf("/pkg/%s?tab=doc", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=overview", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=module", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=versions", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=imports", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=importedby", postgres.SamplePackage.Path),
		fmt.Sprintf("/pkg/%s?tab=licenses", postgres.SamplePackage.Path),
	} {
		t.Run(urlPath, func(t *testing.T) {
			s, err := NewServer(testDB, "../../content/static", false)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}

			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", urlPath, nil))
			if got, want := w.Code, http.StatusOK; got != want {
				t.Fatalf("Code = %d, want %d", got, want)
			}
		})
	}
}
