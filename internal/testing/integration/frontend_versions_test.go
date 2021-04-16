// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/postgres"
)

func TestFrontendVersionsPage(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	exps := []string{
		internal.ExperimentSymbolHistoryVersionsPage,
		internal.ExperimentDoNotInsertNewDocumentation,
	}
	processVersions(
		experiment.NewContext(context.Background(), exps...),
		t, testModules)

	const modulePath = "example.com/symbols"
	for _, test := range []struct {
		name, pkgPath string
		want          []*frontend.VersionList
	}{
		{"versions page symbols - one version all symbols", modulePath, versionsPageSymbols},
		{"versions page hello - multi GOOS", modulePath + "/hello", versionsPageHello},
	} {
		t.Run(test.name, func(t *testing.T) {
			urlPath := fmt.Sprintf("/%s?tab=versions&m=json", test.pkgPath)
			body := getFrontendPage(t, urlPath, exps...)
			var got frontend.VersionsDetails
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if diff := cmp.Diff(test.want, got.ThisModule); diff != "" {
				t.Errorf("mismatch (-want, got):\n%s", diff)
			}
		})
	}
}
