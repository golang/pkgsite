// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package page defines common fields shared by pages when rendering templages.
package page

import (
	"time"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/experiment"
)

// BasePage contains fields shared by all pages when rendering templates.
type BasePage struct {
	// HTMLTitle is the value to use in the page’s <title> tag.
	HTMLTitle string

	// MetaDescription is the html used for rendering the <meta name="Description"> tag.
	MetaDescription safehtml.HTML

	// Query is the current search query (if applicable).
	Query string

	// Experiments contains the experiments currently active.
	Experiments *experiment.Set

	// DevMode indicates whether the server is running in development mode.
	DevMode bool

	// LocalMode indicates whether the server is running in local mode (i.e. ./cmd/pkgsite).
	LocalMode bool

	// AppVersionLabel contains the current version of the app.
	AppVersionLabel string

	// GoogleTagManagerID is the ID used to load Google Tag Manager.
	GoogleTagManagerID string

	// AllowWideContent indicates whether the content should be displayed in a
	// way that’s amenable to wider viewports.
	AllowWideContent bool

	// Enables the two and three column layouts on the unit page.
	UseResponsiveLayout bool

	// SearchPrompt is the prompt/placeholder for search input.
	SearchPrompt string

	// SearchMode is the search mode for the current search request.
	SearchMode string

	// SearchModePackage is the value of const searchModePackage. It is used in
	// the search bar dropdown.
	SearchModePackage string

	// SearchModeSymbol is the value of const searchModeSymbol. It is used in
	// the search bar dropdown.
	SearchModeSymbol string

	// SurveyBanner describes the Go Developer Survey banner to show on the
	// page, or is nil when no banner should be shown.
	SurveyBanner *SurveyBanner
}

// SurveyBanner describes a Go Developer Survey promotion shown on the page.
type SurveyBanner struct {
	// ID uniquely identifies the survey (e.g., "go-survey-2026"). It is placed
	// in the banner element's data-survey-id attribute and used as the
	// localStorage key to persist the banner's minimized state in the browser.
	ID string
	// Name is the human-readable title of the survey (e.g., "2026 Go Developer
	// Survey"), used in the banner link text ("Take the <Name>") and aria-label.
	Name string
	// URL is the target address of the survey link.
	URL string
	// Start and End define the time window [Start, End] during which the banner
	// is displayed in production.
	Start, End time.Time
}

func (p *BasePage) SetBasePage(bp BasePage) {
	bp.SearchMode = p.SearchMode
	*p = bp
}

// ErrorPage contains fields for rendering a HTTP error page.
type ErrorPage struct {
	BasePage
	TemplateName    string
	MessageTemplate template.TrustedTemplate
	MessageData     any
}
