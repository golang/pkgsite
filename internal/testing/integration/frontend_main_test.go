// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

var (
	in      = htmlcheck.In
	hasText = htmlcheck.HasText
)

func TestFrontendMainPage(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx := context.Background()
	processVersions(ctx, t, testModules)

	const modulePath = "example.com/symbols"
	for _, test := range []struct {
		name, pkgPath string
		want          htmlcheck.Checker
	}{
		{
			"main page symbols - one version all symbols",
			modulePath,
			in("",
				in("#F",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#I1",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#I2",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#Int",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#Num",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#S1",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#S2",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#String",
					in(".Documentation-sinceVersion > .Documentation-sinceVersionVersion", hasText("v1.1.0"))),
				in("#T",
					in(".Documentation-sinceVersion", hasText(""))),
				in("#TF",
					in(".Documentation-sinceVersion", hasText(""))),
			),
		},
		{
			"main page hello - multi GOOS default page",
			modulePath + "/hello",
			// Hello is the only symbol when GOOS is not set, so return the
			// empty string instead of v1.2.0.
			// TODO(https://golang.org/issue/37102): decide whether it makes
			// sense to show the version in this case.
			in("", in("#Hello",
				in(".Documentation-sinceVersion", hasText("")))),
		},
		/*
			TODO: fix flaky test and uncomment
			{
				"main page hello - multi GOOS JS page",
				modulePath + "/hello?GOOS=js",
				in("",
					// HelloJS was introduced in v1.1.0, the earliest version of
					// this package. Omit the version, even though it is not the
					// earliest version of the module.
					in("#HelloJS",
						in(".Documentation-sinceVersion", hasText(""))),
					// Hello is not the only symbol when GOOS=js, so show that
					// it was added in v1.2.0.
					in("#Hello",
						in(".Documentation-sinceVersion", hasText("v1.2.0")))),
			},
		*/
	} {
		t.Run(test.name, func(t *testing.T) {
			urlPath := fmt.Sprintf("/%s", test.pkgPath)
			body := getFrontendPage(t, urlPath)
			doc, err := html.Parse(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			if err := test.want(doc); err != nil {
				if testing.Verbose() {
					html.Render(os.Stdout, doc)
				}
				t.Error(err)
			}
		})
	}
}
