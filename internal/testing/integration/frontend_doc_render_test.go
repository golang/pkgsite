// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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
					func F(t time.Time, s string) (T, u) {
						x := 3
						x = C
					}
			`,
			"file2.go": `
					package fdoc

					var V = C
					type T int
					type u int
			`,
		},
	}

	// Process with saving the source.
	processVersions(
		experiment.NewContext(context.Background(), internal.ExperimentUnitPage, internal.ExperimentRemoveUnusedAST),
		t, []*proxy.Module{m})

	workerDoc := getDoc(t, m.ModulePath)
	frontendDoc := getDoc(t, m.ModulePath, internal.ExperimentUnitPage, internal.ExperimentFrontendRenderDoc)
	if diff := cmp.Diff(workerDoc, frontendDoc); diff != "" {
		t.Errorf("mismatch (-worker, +frontend):\n%s", diff)
	}
}

func getDoc(t *testing.T, modulePath string, exps ...string) string {
	ctx := experiment.NewContext(context.Background(),
		append(exps, internal.ExperimentUnitPage)...)
	ts := setupFrontend(ctx, t, nil)
	url := ts.URL + "/" + modulePath
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
	// Remove surrounding whitespace from lines, and blank lines.
	scan := bufio.NewScanner(bytes.NewReader(content))
	var b strings.Builder
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if len(line) > 0 {
			fmt.Fprintln(&b, line)
		}
	}
	if scan.Err() != nil {
		t.Fatal(scan.Err())
	}
	return b.String()
}
