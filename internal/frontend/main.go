// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend/serrors"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware/stats"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// MainDetails contains data needed to render the unit template.
type MainDetails struct {
	// Directories are packages and nested modules relative to the path for the
	// unit.
	Directories []*Directory

	// Licenses contains license metadata used in the header.
	Licenses []LicenseMetadata

	// NumImports is the number of imports for the package.
	NumImports string

	// CommitTime is time that this version was published, or the time that
	// has elapsed since this version was committed if it was done so recently.
	CommitTime string

	// Readme is the rendered readme HTML.
	Readme safehtml.HTML

	// ReadmeOutline is a collection of headings from the readme file
	// used to render the readme outline in the sidebar.
	ReadmeOutline []*Heading

	// ReadmeLinks are from the "Links" section of this unit's readme file, and
	// are displayed on the right sidebar.
	ReadmeLinks []link

	// DocLinks are from the "Links" section of the Go package documentation,
	// and are displayed on the right sidebar.
	DocLinks []link

	// ModuleReadmeLinks are from the "Links" section of this unit's module, if
	// the unit is not itself a module. They are displayed on the right sidebar.
	// See https://golang.org/issue/42968.
	ModuleReadmeLinks []link

	// ImportedByCount is the number of packages that import this path.
	// When the count is > limit it will read as 'limit+'. This field
	// is not supported when using a datasource proxy.
	ImportedByCount string

	DocBody       safehtml.HTML
	DocOutline    safehtml.HTML
	MobileOutline safehtml.HTML
	IsPackage     bool

	// DocSynopsis is used as the content for the <meta name="Description">
	// tag on the main unit page.
	DocSynopsis string

	// GOOS and GOARCH are the build context for the doc.
	GOOS, GOARCH string

	// BuildContexts holds the values for build contexts available for the doc.
	BuildContexts []internal.BuildContext

	// SourceFiles contains .go files for the package.
	SourceFiles []*File

	// RepositoryURL is the URL to the repository containing the package.
	RepositoryURL string

	// SourceURL is the URL to the source of the package.
	SourceURL string

	// ExpandReadme is holds the expandable readme state.
	ExpandReadme bool

	// ModFileURL is an URL to the mod file.
	ModFileURL string

	// IsTaggedVersion is true if the version is not a psuedorelease.
	IsTaggedVersion bool

	// IsStableVersion is true if the major version is v1 or greater.
	IsStableVersion bool
}

// File is a source file for a package.
type File struct {
	Name string
	URL  string
}

func fetchMainDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta,
	requestedVersion string, expandReadme bool, bc internal.BuildContext) (_ *MainDetails, err error) {
	defer stats.Elapsed(ctx, "fetchMainDetails")()

	unit, err := ds.GetUnit(ctx, um, internal.WithMain, bc)
	if err != nil {
		return nil, err
	}
	subdirectories := getSubdirectories(um, unit.Subdirectories, requestedVersion)
	if err != nil {
		return nil, err
	}
	nestedModules, err := getNestedModules(ctx, ds, um, subdirectories)
	if err != nil {
		return nil, err
	}
	readme, err := readmeContent(ctx, unit)
	if err != nil {
		return nil, err
	}
	var (
		docParts           = &dochtml.Parts{}
		docLinks, modLinks []link
		files              []*File
		synopsis           string
		goos, goarch       string
		buildContexts      []internal.BuildContext
	)

	unit.Documentation = cleanDocumentation(unit.Documentation)
	// There should be at most one Documentation.
	var doc *internal.Documentation
	if len(unit.Documentation) > 0 {
		doc = unit.Documentation[0]
	}

	if doc != nil {
		synopsis = doc.Synopsis
		goos = doc.GOOS
		goarch = doc.GOARCH
		buildContexts = unit.BuildContexts
		end := stats.Elapsed(ctx, "DecodePackage")
		docPkg, err := godoc.DecodePackage(doc.Source)
		end()
		if err != nil {
			if errors.Is(err, godoc.ErrInvalidEncodingType) {
				// Instead of returning a 500, return a 404 so the user can
				// reprocess the documentation.
				log.Errorf(ctx, "fetchMainDetails(%q, %q, %q): %v", um.Path, um.ModulePath, um.Version, err)
				return nil, serrors.ErrUnitNotFoundWithoutFetch
			}
			return nil, err
		}

		docParts, err = getHTML(ctx, unit, docPkg, unit.SymbolHistory, bc)
		// If err  is ErrTooLarge, then docBody will have an appropriate message.
		if err != nil && !errors.Is(err, dochtml.ErrTooLarge) {
			return nil, err
		}
		for _, l := range docParts.Links {
			docLinks = append(docLinks, link{Href: l.Href, Body: l.Text})
		}
		end = stats.Elapsed(ctx, "sourceFiles")
		files = sourceFiles(unit, docPkg)
		end()
	}
	// If the unit is not a module, fetch the module readme to extract its
	// links.
	// In the unlikely event that the module is redistributable but the unit is
	// not, we will not show the module links on the unit page.
	if unit.Path != unit.ModulePath && unit.IsRedistributable {
		modReadme, err := ds.GetModuleReadme(ctx, unit.ModulePath, unit.Version)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		if err == nil {
			rm, err := processReadme(ctx, modReadme, um.SourceInfo)
			if err != nil {
				return nil, err
			}
			modLinks = rm.Links
		}
	}

	versionType, err := version.ParseType(um.Version)
	if err != nil {
		return nil, err
	}
	isTaggedVersion := versionType != version.TypePseudo
	isStableVersion := semver.Major(um.Version) != "v0" && versionType == version.TypeRelease
	pr := message.NewPrinter(language.English)
	return &MainDetails{
		ExpandReadme:      expandReadme,
		Directories:       unitDirectories(append(subdirectories, nestedModules...)),
		Licenses:          transformLicenseMetadata(um.Licenses),
		CommitTime:        absoluteTime(um.CommitTime),
		Readme:            readme.HTML,
		ReadmeOutline:     readme.Outline,
		ReadmeLinks:       readme.Links,
		DocLinks:          docLinks,
		ModuleReadmeLinks: modLinks,
		DocOutline:        docParts.Outline,
		DocBody:           docParts.Body,
		DocSynopsis:       synopsis,
		GOOS:              goos,
		GOARCH:            goarch,
		BuildContexts:     buildContexts,
		SourceFiles:       files,
		RepositoryURL:     um.SourceInfo.RepoURL(),
		SourceURL:         um.SourceInfo.DirectoryURL(internal.Suffix(um.Path, um.ModulePath)),
		MobileOutline:     docParts.MobileOutline,
		NumImports:        pr.Sprint(unit.NumImports),
		ImportedByCount:   pr.Sprint(unit.NumImportedBy),
		IsPackage:         unit.IsPackage(),
		ModFileURL:        um.SourceInfo.ModuleURL() + "/go.mod",
		IsTaggedVersion:   isTaggedVersion,
		IsStableVersion:   isStableVersion,
	}, nil
}

func cleanDocumentation(docs []*internal.Documentation) []*internal.Documentation {
	// If there is more than one row but the first is all/all, ignore the others.
	// Should never happen;  temporary fix until the DB is cleaned up.
	if len(docs) > 1 && docs[0].BuildContext() == internal.BuildContextAll {
		return docs[:1]
	}
	// If there is only one Documentation and it is linux/amd64, then
	// make it all/all.
	//
	// This is temporary, until the next reprocessing. It assumes a unit
	// with a single linux/amd64 actually has only one build context,
	// and hasn't been reprocessed to have all/all.
	//
	// The only effect of this is to prevent "GOOS=linux, GOARCH=amd64" from
	// appearing at the bottom of the doc. That is wrong in the (rather
	// unlikely) case that the package truly only has doc for linux/amd64,
	// but the bug is only cosmetic.
	if len(docs) == 1 && docs[0].GOOS == "linux" && docs[0].GOARCH == "amd64" {
		docs[0].GOOS = internal.All
		docs[0].GOARCH = internal.All
	}
	return docs
}

// readmeContent renders the readme to html and collects the headings
// into an outline.
func readmeContent(ctx context.Context, u *internal.Unit) (_ *Readme, err error) {
	defer derrors.Wrap(&err, "readmeContent(%q, %q, %q)", u.Path, u.ModulePath, u.Version)
	defer stats.Elapsed(ctx, "readmeContent")()
	if !u.IsRedistributable {
		return &Readme{}, nil
	}
	return ProcessReadmeMarkdown(ctx, u)
}

const missingDocReplacement = `<p>Documentation is missing.</p>`

func getHTML(ctx context.Context, u *internal.Unit, docPkg *godoc.Package,
	nameToVersion map[string]string, bc internal.BuildContext) (_ *dochtml.Parts, err error) {
	defer derrors.Wrap(&err, "getHTML(%s)", u.Path)

	if len(u.Documentation[0].Source) > 0 {
		return renderDocParts(ctx, u, docPkg, nameToVersion, bc)
	}
	log.Errorf(ctx, "unit %s (%s@%s) missing documentation source", u.Path, u.ModulePath, u.Version)
	return &dochtml.Parts{Body: template.MustParseAndExecuteToHTML(missingDocReplacement)}, nil
}
