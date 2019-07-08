// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

func TestHTMLInjection(t *testing.T) {
	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

func TestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	sampleVersion := sample.Version()
	if err := testDB.InsertVersion(ctx, sampleVersion, sample.Licenses); err != nil {
		t.Fatalf("db.InsertVersion(%+v): %v", sampleVersion, err)
	}
	if err := testDB.InsertDocuments(ctx, sampleVersion); err != nil {
		t.Fatalf("testDB.InsertDocument(%+v): %v", sampleVersion, err)
	}
	testDB.RefreshSearchDocuments(ctx)

	for _, urlPath := range []string{
		"/static/",
		"/license-policy",
		"/favicon.ico",
		fmt.Sprintf("/search?q=%s", sample.PackageName),
		fmt.Sprintf("/pkg/%s", sample.PackagePath),
		fmt.Sprintf("/pkg/%s@%s", sample.PackagePath, sample.VersionString),
		fmt.Sprintf("/pkg/%s?tab=doc", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=overview", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=module", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=versions", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=imports", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=importedby", sample.PackagePath),
		fmt.Sprintf("/pkg/%s?tab=licenses", sample.PackagePath),
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
