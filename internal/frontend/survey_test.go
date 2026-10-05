// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	pagepkg "golang.org/x/pkgsite/internal/frontend/page"
)

func TestSurveyJSON(t *testing.T) {
	if len(bytes.TrimSpace(surveyJSON)) == 0 {
		return
	}
	var b pagepkg.SurveyBanner
	if err := json.Unmarshal(surveyJSON, &b); err != nil {
		t.Fatalf("parsing survey.json: %v", err)
	}
	if b.ID == "" || b.Name == "" || b.URL == "" {
		t.Errorf("survey.json has empty required string field: %+v", b)
	}
	if _, err := url.ParseRequestURI(b.URL); err != nil {
		t.Errorf("survey.json has invalid URL %q: %v", b.URL, err)
	}
	if b.Start.IsZero() || b.End.IsZero() || !b.End.After(b.Start) {
		t.Errorf("survey.json has invalid time range [%v, %v]", b.Start, b.End)
	}
}

func TestServerSurveyBanner(t *testing.T) {
	survey := pagepkg.SurveyBanner{
		ID:    "go-survey-test",
		Name:  "Test Go Developer Survey",
		URL:   "https://go.dev/survey",
		Start: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
	}
	before := survey.Start.Add(-time.Hour)
	inside := survey.Start.Add(time.Hour)
	after := survey.End.Add(time.Hour)
	for _, tc := range []struct {
		name string
		s    *Server
		now  time.Time
		want bool
	}{
		{"prod before start", &Server{survey: survey, deploymentEnvironment: "prod"}, before, false},
		{"prod inside window", &Server{survey: survey, deploymentEnvironment: "prod"}, inside, true},
		{"prod after end", &Server{survey: survey, deploymentEnvironment: "prod"}, after, false},
		{"staging before start", &Server{survey: survey, deploymentEnvironment: "staging"}, before, true},
		{"staging inside window", &Server{survey: survey, deploymentEnvironment: "staging"}, inside, true},
		{"staging after end", &Server{survey: survey, deploymentEnvironment: "staging"}, after, false},
		{"staging disabled", &Server{survey: survey, deploymentEnvironment: "staging", disableTemporaryBanner: true}, before, false},
		{"devMode before start", &Server{survey: survey, deploymentEnvironment: "prod", devMode: true}, before, true},
		{"devMode after end", &Server{survey: survey, deploymentEnvironment: "prod", devMode: true}, after, false},
		{"disabled", &Server{survey: survey, deploymentEnvironment: "prod", disableTemporaryBanner: true}, inside, false},
		{"local mode", &Server{survey: survey, deploymentEnvironment: "prod", localMode: true}, inside, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.surveyBanner(tc.now); (got != nil) != tc.want {
				t.Errorf("surveyBanner(%v) = %v, want active=%v", tc.now, got, tc.want)
			}
		})
	}
}
