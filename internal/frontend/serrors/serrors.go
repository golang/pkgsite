// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// serrors contains error types used by the server
package serrors

import (
	"fmt"
	"net/http"

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
