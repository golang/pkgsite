// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
)

const (
	latestMinorClassPlaceholder   = "$$GODISCOVERY_LATESTMINORCLASS$$"
	LatestMinorVersionPlaceholder = "$$GODISCOVERY_LATESTMINORVERSION$$"
	latestMajorClassPlaceholder   = "$$GODISCOVERY_LATESTMAJORCLASS$$"
	LatestMajorVersionPlaceholder = "$$GODISCOVERY_LATESTMAJORVERSION$$"
	LatestMajorVersionURL         = "$$GODISCOVERY_LATESTMAJORVERSIONURL$$"
)

// latestInfoRegexp extracts values needed to determine the latest-version badge from a page's HTML.
var latestInfoRegexp = regexp.MustCompile(`data-version="([^"]*)" data-mpath="([^"]*)" data-ppath="([^"]*)" data-pagetype="([^"]*)"`)

type latestMinorFunc func(ctx context.Context, packagePath, modulePath, pageType string) string
type latestMajorFunc func(ctx context.Context, seriesPath string) string

// LatestVersions replaces the HTML placeholder values for the badge and banner
// that displays whether the version of the package or module being served is
// the latest minor version (badge) and the latest major version (banner).
func LatestVersions(latestMinor latestMinorFunc, latestMajor latestMajorFunc) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			crw := &capturingResponseWriter{ResponseWriter: w}
			h.ServeHTTP(crw, r)
			body := crw.bytes()
			matches := latestInfoRegexp.FindSubmatch(body)
			if matches != nil {
				version := string(matches[1])
				// The template package converts '+' to its HTML entity.
				version = strings.Replace(version, "&#43;", "+", -1)
				modulePath := string(matches[2])
				seriesPath := internal.SeriesPathForModule(modulePath)
				_, majorVersion, _ := module.SplitPathVersion(modulePath)
				packagePath := string(matches[3])
				pageType := string(matches[4])
				latestMinorVersion := latestMinor(r.Context(), packagePath, modulePath, pageType)
				latestMinorClass := "DetailsHeader-badge"
				switch {
				case latestMinorVersion == "":
					latestMinorClass += "--unknown"
				case latestMinorVersion == version:
					latestMinorClass += "--latest"
				default:
					latestMinorClass += "--goToLatest"
				}
				latestMajorVersion := latestMajor(r.Context(), seriesPath)
				latestMajorClass := ""
				// If the latest major version is the same as the major version of the current
				// module path, it is currently the latest version so we don't show the banner.
				// If an error occurs finding a major version (i.e: not found) an empty string
				// is returned in which case we also don't show the banner.
				if majorVersion == latestMajorVersion || latestMajorVersion == "" {
					latestMajorClass += " DetailsHeader-banner--latest"
				}
				body = bytes.ReplaceAll(body, []byte(latestMinorClassPlaceholder), []byte(latestMinorClass))
				body = bytes.ReplaceAll(body, []byte(LatestMinorVersionPlaceholder), []byte(latestMinorVersion))
				body = bytes.ReplaceAll(body, []byte(latestMajorClassPlaceholder), []byte(latestMajorClass))
				body = bytes.ReplaceAll(body, []byte(LatestMajorVersionPlaceholder), []byte(latestMajorVersion))
				body = bytes.ReplaceAll(body, []byte(LatestMajorVersionURL), []byte(seriesPath+latestMajorVersion))
			}
			if _, err := w.Write(body); err != nil {
				log.Errorf(r.Context(), "LatestVersions, writing: %v", err)
			}
		})
	}
}
