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
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres"
)

// Test that the worker saves the information needed to render
// doc on the frontend.
func TestFrontendDocRender(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	// Process with saving the source.
	processVersions(
		experiment.NewContext(context.Background()),
		t, testModules)

	const modulePath = "example.com/basic"

	workerDoc := getDoc(t, modulePath)
	frontendDoc := getDoc(t, modulePath)
	if diff := cmp.Diff(workerDoc, frontendDoc); diff != "" {
		t.Errorf("mismatch (-worker, +frontend):\n%s", diff)
	}
}

func getDoc(t *testing.T, modulePath string, exps ...string) string {
	ctx := experiment.NewContext(context.Background(), exps...)
	ts := setupFrontend(ctx, t, nil, nil)
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
