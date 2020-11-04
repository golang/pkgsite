// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestReadme(t *testing.T) {
	ctx := experiment.NewContext(context.Background(), internal.ExperimentGoldmark)
	mod := &internal.ModuleInfo{
		Version:    sample.VersionString,
		SourceInfo: source.NewGitHubInfo(sample.ModulePath, "", sample.VersionString),
	}
	for _, tc := range []struct {
		name        string
		mi          *internal.ModuleInfo
		readme      *internal.Readme
		wantHTML    string
		wantOutline []*Heading
	}{
		{
			name: "Top level heading of h4 becomes h3, and following header levels become hN-1",
			mi:   mod,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "#### Heading Rank 4\n\n##### Heading Rank 5",
			},
			wantHTML: "<h3 class=\"h4\" id=\"heading-rank-4\">Heading Rank 4</h3>\n<h4 class=\"h5\" id=\"heading-rank-5\">Heading Rank 5</h4>",
			wantOutline: []*Heading{
				{Level: 4, Text: "Heading Rank 4", ID: "heading-rank-4"},
				{Level: 5, Text: "Heading Rank 5", ID: "heading-rank-5"},
			},
		},
		{
			name: "Github markdown emoji markup is properly rendered",
			mi:   mod,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "# :zap: Zap \n\n :joy:",
			},
			wantHTML: "<h3 class=\"h1\" id=\"zap-zap\">⚡ Zap</h3>\n<p>😂</p>",
			wantOutline: []*Heading{
				{Level: 1, Text: " Zap", ID: "zap-zap"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unit := internal.Unit{
				Readme: tc.readme,
				UnitMeta: internal.UnitMeta{
					SourceInfo: tc.mi.SourceInfo,
				},
			}
			html, gotOutline, err := Readme(ctx, &unit)
			if err != nil {
				t.Fatal(err)
			}
			gotHTML := strings.TrimSpace(html.String())
			if diff := cmp.Diff(tc.wantHTML, gotHTML); diff != "" {
				t.Errorf("Readme(%v) html mismatch (-want +got):\n%s", tc.mi, diff)
			}
			if diff := cmp.Diff(tc.wantOutline, gotOutline); diff != "" {
				t.Errorf("Readme(%v) outline mismatch (-want +got):\n%s", tc.mi, diff)
			}
		})
	}
}
