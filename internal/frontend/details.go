// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/pkgsite/internal/frontend/page"
	"golang.org/x/pkgsite/internal/frontend/serrors"
	"golang.org/x/pkgsite/internal/frontend/urlinfo"
	mstats "golang.org/x/pkgsite/internal/middleware/stats"

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
		return &serrors.ServerError{Status: http.StatusMethodNotAllowed}
	}
	// 站点根：原版只判 "/"，挂 -base-path 后还要判 basePath 自身（带或不带尾斜杠）。
	if r.URL.Path == "/" || r.URL.Path == s.basePath || r.URL.Path == s.basePath+"/" {
		s.serveHomepage(ctx, w, r)
		return nil
	}
	// trailing-slash 规范化：去尾再 301，但不要把 basePath 自身的尾去掉
	// （否则跟 mux 自动 redirect 形成 301 ↔ 307 死循环）。
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

	// 给 ExtractURLPathInfo 看到的 path 剥掉 basePath——它假设挂根，
	// path 形如 "/<module>[@<ver>]"，basePath 不属于 module path 一部分。
	parsePath := r.URL.Path
	if s.basePath != "" {
		parsePath = strings.TrimPrefix(parsePath, s.basePath)
	}
	urlInfo, err := urlinfo.ExtractURLPathInfo(parsePath)
	if err != nil {
		var epage *page.ErrorPage
		if uerr := new(urlinfo.UserError); errors.As(err, &uerr) {
			epage = &page.ErrorPage{MessageData: uerr.UserMessage}
		}
		return &serrors.ServerError{
			Status: http.StatusBadRequest,
			Err:    err,
			Epage:  epage,
		}
	}
	if !urlinfo.IsSupportedVersion(urlInfo.FullPath, urlInfo.RequestedVersion) {
		return serrors.InvalidVersionError(urlInfo.FullPath, urlInfo.RequestedVersion)
	}
	if urlPath := stdlibRedirectURL(urlInfo.FullPath); urlPath != "" {
		// stdlibRedirectURL 返回 "/std" 之类挂根 path，反代场景要补 basePath。
		http.Redirect(w, r, s.basePath+urlPath, http.StatusMovedPermanently)
		return
	}
	if err := checkExcluded(ctx, ds, urlInfo.FullPath, urlInfo.RequestedVersion); err != nil {
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

func checkExcluded(ctx context.Context, ds internal.DataSource, fullPath, version string) error {
	db, ok := ds.(internal.PostgresDB)
	if !ok {
		return nil
	}
	if db.IsExcluded(ctx, fullPath, version) {
		// Return NotFound; don't let the user know that the package was excluded.
		return &serrors.ServerError{Status: http.StatusNotFound}
	}
	return nil
}
