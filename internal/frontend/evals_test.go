// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestFetchEvalsDetails(t *testing.T) {
	ctx := context.Background()

	mit := &licenses.Metadata{Types: []string{"MIT"}, FilePath: "LICENSE"}
	mitLicense := &licenses.License{
		Metadata: mit,
		Contents: []byte("MIT License"),
	}

	tests := []struct {
		name              string
		version           string
		isRedistributable bool
		licenses          []*licenses.License
		want              []eval
	}{
		{
			name:              "no license, untagged pseudo version",
			version:           "v0.0.0-20260101120000-abcdef123456",
			isRedistributable: false,
			licenses:          nil,
			want: []eval{
				{
					Type:  licenseEval,
					Score: 0,
					Value: "no license",
				},
				{
					Type:  moduleVersionEval,
					Score: 0,
					Value: "untagged",
				},
			},
		},
		{
			name:              "no license, prerelease",
			version:           "v1.2.3-alpha",
			isRedistributable: false,
			licenses:          nil,
			want: []eval{
				{
					Type:  licenseEval,
					Score: 0,
					Value: "no license",
				},
				{
					Type:  moduleVersionEval,
					Score: 1,
					Value: "tagged, unstable",
				},
			},
		},
		{
			name:              "non-redistributable license, unstable v0 tagged version",
			version:           "v0.1.0",
			isRedistributable: false,
			licenses:          []*licenses.License{mitLicense},
			want: []eval{
				{
					Type:  licenseEval,
					Score: 1,
					Value: "non-redistributable license",
				},
				{
					Type:  moduleVersionEval,
					Score: 1,
					Value: "tagged, unstable",
				},
			},
		},
		{
			name:              "redistributable license, stable v1 tagged version",
			version:           "v1.2.3",
			isRedistributable: true,
			licenses:          []*licenses.License{mitLicense},
			want: []eval{
				{
					Type:  licenseEval,
					Score: 2,
					Value: "redistributable license",
				},
				{
					Type:  moduleVersionEval,
					Score: 2,
					Value: "tagged, stable",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fds := fakedatasource.New()
			mod := sample.Module(sample.ModulePath, tt.version, sample.Suffix)
			mod.IsRedistributable = tt.isRedistributable
			mod.Licenses = tt.licenses
			for _, u := range mod.Units {
				u.IsRedistributable = tt.isRedistributable
				u.LicenseContents = tt.licenses
			}
			fds.MustInsertModule(t, mod)

			um := &internal.UnitMeta{
				Path:       sample.ModulePath + "/" + sample.Suffix,
				ModuleInfo: mod.ModuleInfo,
			}

			got, err := fetchEvalsDetails(ctx, fds, um)
			if err != nil {
				t.Fatalf("fetchEvalsDetails() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got.Evals, cmp.AllowUnexported(eval{}, evalType{})); diff != "" {
				t.Errorf("fetchEvalsDetails() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
