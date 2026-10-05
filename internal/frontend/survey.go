// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	_ "embed"
	"time"

	pagepkg "golang.org/x/pkgsite/internal/frontend/page"
)

//go:embed survey.json
var surveyJSON []byte

// surveyBanner returns the survey banner to show on a page served at time
// now, or nil if the banner is disabled or no survey is open.
// The banner is never shown in local mode (cmd/pkgsite).
// Outside prod, the start time check is skipped so the banner can be
// previewed before the survey opens.
func (s *Server) surveyBanner(now time.Time) *pagepkg.SurveyBanner {
	if s.disableTemporaryBanner || s.localMode {
		return nil
	}
	if now.After(s.survey.End) {
		return nil
	}
	if !s.devMode && s.deploymentEnvironment == "prod" && now.Before(s.survey.Start) {
		return nil
	}
	return &s.survey
}
