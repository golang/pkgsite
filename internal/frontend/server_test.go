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
	if err := testDB.InsertVersion(ctx, sampleInternalVersion, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%+v): %v", sampleInternalVersion, err)
	}
	if err := testDB.InsertDocuments(ctx, sampleInternalVersion); err != nil {
		t.Fatalf("testDB.InsertDocument(%+v): %v", sampleInternalVersion, err)
	}
	testDB.RefreshSearchDocuments(ctx)

	for _, urlPath := range []string{
		"/static/",
		"/license-policy/",
		"/favicon.ico",
		fmt.Sprintf("/search/?q=%s", sampleInternalPackage.Name),
		fmt.Sprintf("/%s", sampleInternalPackage.Path),
		fmt.Sprintf("/%s@%s", sampleInternalPackage.Path, sampleInternalVersion.Version),
		fmt.Sprintf("/%s?tab=doc", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=overview", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=module", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=versions", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=imports", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=importedby", sampleInternalPackage.Path),
		fmt.Sprintf("/%s?tab=licenses", sampleInternalPackage.Path),
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
