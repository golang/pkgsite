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

	"golang.org/x/pkgsite/internal/log"
)

const (
	latestMinorClassPlaceholder   = "$$GODISCOVERY_LATESTMINORCLASS$$"
	LatestMinorVersionPlaceholder = "$$GODISCOVERY_LATESTMINORVERSION$$"
)

// latestInfoRegexp extracts values needed to determine the latest-version badge from a page's HTML.
var latestInfoRegexp = regexp.MustCompile(`data-version="([^"]*)" data-mpath="([^"]*)" data-ppath="([^"]*)" data-pagetype="([^"]*)"`)

type latestFunc func(ctx context.Context, packagePath, modulePath, pageType string) string

// LatestVersion supports the badge that displays whether the version of the
// package or module being served is the latest one.
func LatestVersion(latest latestFunc) Middleware {
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
				packagePath := string(matches[3])
				pageType := string(matches[4])
				latestMinorVersion := latest(r.Context(), packagePath, modulePath, pageType)
				latestMinorClass := "DetailsHeader-badge"
				switch {
				case latestMinorVersion == "":
					latestMinorClass += "--unknown"
				case latestMinorVersion == version:
					latestMinorClass += "--latest"
				default:
					latestMinorClass += "--goToLatest"
				}
				body = bytes.ReplaceAll(body, []byte(latestMinorClassPlaceholder), []byte(latestMinorClass))
				body = bytes.ReplaceAll(body, []byte(LatestMinorVersionPlaceholder), []byte(latestMinorVersion))
			}
			if _, err := w.Write(body); err != nil {
				log.Errorf(r.Context(), "LatestVersion, writing: %v", err)
			}
		})
	}
}
