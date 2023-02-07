// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"encoding/json"
	"go/format"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"golang.org/x/pkgsite/internal/log"
)

// playgroundURL is the playground endpoint used for share links.
var playgroundURL = &url.URL{Scheme: "https", Host: "play.golang.org"}

func httpErrorStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// proxyPlayground is a handler that proxies playground requests to play.golang.org.
func (s *Server) proxyPlayground(w http.ResponseWriter, r *http.Request) {
	makePlaygroundProxy(playgroundURL).ServeHTTP(w, r)
}

// makePlaygroundProxy creates a proxy that sends requests to play.golang.org.
// The prefix /play is removed from the URL path.
func makePlaygroundProxy(pgURL *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.Header.Add("X-Forwarded-Host", req.Host)
			req.Header.Add("X-Origin-Host", pgURL.Host)
			req.Host = pgURL.Host
			req.URL.Scheme = pgURL.Scheme
			req.URL.Host = pgURL.Host
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/play")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Errorf(r.Context(), "ERROR playground proxy error: %v", err)
			httpErrorStatus(w, http.StatusInternalServerError)
		},
	}
}

type fmtResponse struct {
	Body  string
	Error string
}

// handleFmt takes a Go program in its "body" form value, formats it with
// standard gofmt formatting, and writes a fmtResponse as a JSON object.
func (s *Server) handleFmt(w http.ResponseWriter, r *http.Request) {
	resp := new(fmtResponse)
	body, err := format.Source([]byte(r.FormValue("body")))
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Body = string(body)
	}
	w.Header().Set("Content-type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}
