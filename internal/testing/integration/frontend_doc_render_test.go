// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

// Test that the worker saves the information needed to render
// doc on the frontend.
func TestFrontendDocRender(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	m := &proxy.Module{
		ModulePath: "github.com/golang/fdoc",
		Version:    "v1.2.3",
		Files: map[string]string{
			"go.mod":  "module github.com/golang/fdoc",
			"LICENSE": testhelper.MITLicense,
			"file.go": `
					// Package fdoc is a test of frontend doc rendering.
					package fdoc

					import 	"time"

					// C is a constant.
					const C = 123

					// F is a function.
					func F(t time.Time, s string) T {
						_ = C
					}
			`,
			"file2.go": `
					package fdoc

					var V = C
					type T int
			`,
		},
	}

	commonExps := []string{
		internal.ExperimentRemoveUnusedAST,
		internal.ExperimentInsertPackageSource,
		internal.ExperimentSidenav,
		internal.ExperimentUnitPage,
	}

	ctx := experiment.NewContext(context.Background(), commonExps...)
	processVersions(ctx, t, []*proxy.Module{m})

	getDoc := func(exps ...string) string {
		t.Helper()

		ectx := experiment.NewContext(ctx, append(exps, commonExps...)...)
		ts := setupFrontend(ectx, t, nil)
		url := ts.URL + "/" + m.ModulePath
		return getPage(ectx, t, url)
	}

	workerDoc := getDoc()
	frontendDoc := getDoc(internal.ExperimentFrontendRenderDoc)

	if diff := cmp.Diff(workerDoc, frontendDoc); diff != "" {
		t.Errorf("mismatch (-worker, +frontend):\n%s", diff)
	}
}

func getPage(ctx context.Context, t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("%s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: status code %d", url, resp.StatusCode)
	}
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s: %v", url, err)
	}
	return string(content)
}
