// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// serrors contains error types used by the server
package serrors

import (
	"fmt"
	"net/http"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/frontend/page"
)

// ServerError is a type of error that can be dosplayed by the server.
type ServerError struct {
	Status       int    // HTTP status code
	ResponseText string // Response text to the user
	Epage        *page.ErrorPage
	Err          error // wrapped error
}

func (s *ServerError) Error() string {
	return fmt.Sprintf("%d (%s): %v (epage=%v)", s.Status, http.StatusText(s.Status), s.Err, s.Epage)
}

func (s *ServerError) Unwrap() error {
	return s.Err
}

func DatasourceNotSupportedError() error {
	return &ServerError{
		Status: http.StatusFailedDependency,
		Epage: &page.ErrorPage{
			MessageTemplate: template.MakeTrustedTemplate(
				`<h3 class="Error-message">This page is not supported by this datasource.</h3>`),
		},
	}
}

func InvalidVersionError(fullPath, requestedVersion string) error {
	return &ServerError{
		Status: http.StatusBadRequest,
		Epage: &page.ErrorPage{
			MessageTemplate: template.MakeTrustedTemplate(`
					<h3 class="Error-message">{{.Version}} is not a valid semantic version.</h3>
					<p class="Error-message">
					  To search for packages like {{.Path}}, <a href="/search?q={{.Path}}">click here</a>.
					</p>`),
			MessageData: struct{ Path, Version string }{fullPath, requestedVersion},
		},
	}
}

// errUnitNotFoundWithoutFetch returns a 404 with instructions to the user on
// how to manually fetch the package. No fetch button is provided. This is used
// for very large modules or modules that previously 500ed.
var ErrUnitNotFoundWithoutFetch = &ServerError{
	Status: http.StatusNotFound,
	Epage: &page.ErrorPage{
		MessageTemplate: template.MakeTrustedTemplate(`
					    <h3 class="Error-message">{{.StatusText}}</h3>
					    <p class="Error-message">Check that you entered the URL correctly or try fetching it following the
                        <a href="/about#adding-a-package">instructions here</a>.</p>`),
		MessageData: struct{ StatusText string }{http.StatusText(http.StatusNotFound)},
	},
}
