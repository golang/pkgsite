// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"net/http"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	hc "golang.org/x/pkgsite/internal/testing/htmlcheck"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

// Test that the worker saves the information needed to render
// doc on the frontend.
func TestFrontendDocRender(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx := experiment.NewContext(context.Background(),
		internal.ExperimentRemoveUnusedAST,
		internal.ExperimentInsertPackageSource,
		internal.ExperimentFrontendRenderDoc)
	m := &proxy.Module{
		ModulePath: "github.com/golang/fdoc",
		Version:    "v1.2.3",
		Files: map[string]string{
			"go.mod":  "module github.com/golang/fdoc",
			"LICENSE": testhelper.MITLicense,
			"file.go": `
					// Package fdoc is a test of frontend doc rendering.
					package fdoc

					// C is a constant.
					const C = 123

					// F is a function.
					func F() {
						// lots of code
						print(1, 2, 3)
						_ = C
						// more
					}
			`,
		},
	}

	ts := setupFrontend(ctx, t, nil)
	processVersions(ctx, t, []*proxy.Module{m})

	validateResponse(t, http.MethodGet, ts.URL+"/"+m.ModulePath, 200,
		hc.In(".Documentation-content",
			hc.In(".Documentation-overview", hc.HasText("Package fdoc is a test of frontend doc rendering.")),
			hc.In(".Documentation-constants", hc.HasText("C is a constant.")),
			hc.In(".Documentation-functions",
				hc.In("a",
					hc.HasHref("https://github.com/golang/fdoc/blob/v1.2.3/file.go#L9"),
					hc.HasText("F")),
				hc.In("pre", hc.HasExactText("func F()")),
				hc.In("p", hc.HasText("F is a function.")))))

}
