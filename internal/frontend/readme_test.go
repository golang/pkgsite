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

func TestGoldmarkReadmeHTML(t *testing.T) {
	ctx := experiment.NewContext(context.Background(), internal.ExperimentGoldmark)
	mod := &internal.ModuleInfo{
		Version:    sample.VersionString,
		SourceInfo: source.NewGitHubInfo(sample.ModulePath, "", sample.VersionString),
	}
	for _, tc := range []struct {
		name   string
		mi     *internal.ModuleInfo
		readme *internal.Readme
		want   string
	}{
		{
			name: "Top level heading is h3 from ####, and following header levels become hN-1",
			mi:   mod,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "#### Heading Rank 4\n\n##### Heading Rank 5",
			},
			want: "<h3 class=\"h4\" id=\"heading-rank-4\">Heading Rank 4</h3>\n<h4 class=\"h5\" id=\"heading-rank-5\">Heading Rank 5</h4>",
		},
		{
			name: "Github markdown emoji markup is properly rendered",
			mi:   mod,
			readme: &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: "# :zap: Zap \n\n :joy:",
			},
			want: "<h3 class=\"h1\" id=\"zap-zap\">⚡ Zap</h3>\n<p>😂</p>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hgot, err := ReadmeHTML(ctx, tc.mi, tc.readme)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(hgot.String())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ReadmeHTML(%v) mismatch (-want +got):\n%s", tc.mi, diff)
			}
		})
	}
}
