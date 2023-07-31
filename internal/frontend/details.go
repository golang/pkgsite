// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	mstats "golang.org/x/pkgsite/internal/middleware/stats"
	"net/http"
	"strings"

	"github.com/google/safehtml/template"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// serveDetails handles requests for package/directory/module details pages. It
// expects paths of the form "/<module-path>[@<version>?tab=<tab>]".
// stdlib module pages are handled at "/std", and requests to "/mod/std" will
// be redirected to that path.
func (s *Server) serveDetails(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer mstats.Elapsed(r.Context(), "serveDetails")()

	ctx := r.Context()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return &serverError{status: http.StatusMethodNotAllowed}
	}
	if r.URL.Path == "/" {
		s.serveHomepage(ctx, w, r)
		return nil
	}
	if strings.HasSuffix(r.URL.Path, "/") {
		url := *r.URL
		url.Path = strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, url.String(), http.StatusMovedPermanently)
		return
	}

	// If page statistics are enabled, use the "exp" query param to adjust
	// the active experiments.
	if s.serveStats {
		ctx = setExperimentsFromQueryParam(ctx, r)
	}

	urlInfo, err := extractURLPathInfo(r.URL.Path)
	if err != nil {
		var epage *errorPage
		if uerr := new(userError); errors.As(err, &uerr) {
			epage = &errorPage{MessageData: uerr.userMessage}
		}
		return &serverError{
			status: http.StatusBadRequest,
			err:    err,
			epage:  epage,
		}
	}
	if !isSupportedVersion(urlInfo.fullPath, urlInfo.requestedVersion) {
		return invalidVersionError(urlInfo.fullPath, urlInfo.requestedVersion)
	}
	if urlPath := stdlibRedirectURL(urlInfo.fullPath); urlPath != "" {
		http.Redirect(w, r, urlPath, http.StatusMovedPermanently)
		return
	}
	if err := checkExcluded(ctx, ds, urlInfo.fullPath); err != nil {
		return err
	}
	return s.serveUnitPage(ctx, w, r, ds, urlInfo)
}

func stdlibRedirectURL(fullPath string) string {
	if !strings.HasPrefix(fullPath, stdlib.GitHubRepo) {
		return ""
	}
	if fullPath == stdlib.GitHubRepo || fullPath == stdlib.GitHubRepo+"/src" {
		return "/std"
	}
	urlPath2 := strings.TrimPrefix(strings.TrimPrefix(fullPath, stdlib.GitHubRepo+"/"), "src/")
	if fullPath == urlPath2 {
		return ""
	}
	return "/" + urlPath2
}

func invalidVersionError(fullPath, requestedVersion string) error {
	return &serverError{
		status: http.StatusBadRequest,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(`
					<h3 class="Error-message">{{.Version}} is not a valid semantic version.</h3>
					<p class="Error-message">
					  To search for packages like {{.Path}}, <a href="/search?q={{.Path}}">click here</a>.
					</p>`),
			MessageData: struct{ Path, Version string }{fullPath, requestedVersion},
		},
	}
}

func datasourceNotSupportedErr() error {
	return &serverError{
		status: http.StatusFailedDependency,
		epage: &errorPage{
			messageTemplate: template.MakeTrustedTemplate(
				`<h3 class="Error-message">This page is not supported by this datasource.</h3>`),
		},
	}
}

var (
	keyVersionType     = tag.MustNewKey("frontend.version_type")
	versionTypeResults = stats.Int64(
		"go-discovery/frontend_version_type_count",
		"The version type of a request to package, module, or directory page.",
		stats.UnitDimensionless,
	)
	VersionTypeCount = &view.View{
		Name:        "go-discovery/frontend_version_type/result_count",
		Measure:     versionTypeResults,
		Aggregation: view.Count(),
		Description: "version type results, by latest, master, or semver",
		TagKeys:     []tag.Key{keyVersionType},
	}
)

func recordVersionTypeMetric(ctx context.Context, requestedVersion string) {
	// Tag versions based on latest, master and semver.
	v := requestedVersion
	if semver.IsValid(v) {
		v = "semver"
	}
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyVersionType, v),
	}, versionTypeResults.M(1))
}
