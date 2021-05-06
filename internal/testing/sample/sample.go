// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"math"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	oldlicensecheck "github.com/google/licensecheck/old"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

// These sample values can be used to construct test cases.
var (
	ModulePath                = "github.com/valid/module_name"
	RepositoryURL             = "https://github.com/valid/module_name"
	VersionString             = "v1.0.0"
	CommitTime                = NowTruncated()
	LicenseType               = "MIT"
	LicenseFilePath           = "LICENSE"
	NonRedistributableLicense = &licenses.License{
		Metadata: &licenses.Metadata{
			FilePath: "NONREDIST_LICENSE",
			Types:    []string{"UNKNOWN"},
		},
		Contents: []byte(`unknown`),
	}
	PackageName    = "foo"
	Suffix         = "foo"
	PackagePath    = path.Join(ModulePath, Suffix)
	V1Path         = PackagePath
	ReadmeFilePath = "README.md"
	ReadmeContents = "readme"
	GOOS           = internal.All
	GOARCH         = internal.All
	Doc            = Documentation(GOOS, GOARCH, DocContents)
	DocContents    = `
		// Package p is a package.
		//
		//
		// Links
		//
		// - pkg.go.dev, https://pkg.go.dev
 		package p
		var V int
	`
	Constant = &internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:     "Constant",
			Synopsis: "const Constant",
			Section:  internal.SymbolSectionConstants,
			Kind:     internal.SymbolKindConstant,
		},
		GOOS:   internal.All,
		GOARCH: internal.All,
	}
	Variable = &internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:     "Variable",
			Synopsis: "var Variable",
			Section:  internal.SymbolSectionVariables,
			Kind:     internal.SymbolKindVariable,
		},
		GOOS:   internal.All,
		GOARCH: internal.All,
	}
	Function = &internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:     "Function",
			Synopsis: "func Function() error",
			Section:  internal.SymbolSectionFunctions,
			Kind:     internal.SymbolKindFunction,
		},
		GOOS:   internal.All,
		GOARCH: internal.All,
	}
	FunctionNew = &internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:       "New",
			Synopsis:   "func New() *Type",
			Section:    internal.SymbolSectionTypes,
			Kind:       internal.SymbolKindFunction,
			ParentName: "Type",
		},
		GOOS:   internal.All,
		GOARCH: internal.All,
	}
	Type = &internal.Symbol{
		SymbolMeta: internal.SymbolMeta{
			Name:     "Type",
			Synopsis: "type Type struct",
			Section:  internal.SymbolSectionTypes,
			Kind:     internal.SymbolKindType,
		},
		GOOS:   internal.All,
		GOARCH: internal.All,
		Children: []*internal.SymbolMeta{
			func() *internal.SymbolMeta {
				n := FunctionNew.SymbolMeta
				return &n
			}(),
			{
				Name:       "Type.Field",
				Synopsis:   "field",
				Section:    internal.SymbolSectionTypes,
				Kind:       internal.SymbolKindField,
				ParentName: "Type",
			},
			{
				Name:       "Type.Method",
				Synopsis:   "method",
				Section:    internal.SymbolSectionTypes,
				Kind:       internal.SymbolKindMethod,
				ParentName: "Type",
			},
		},
	}
)

// LicenseCmpOpts are options to use when comparing licenses with the cmp package.
var LicenseCmpOpts = []cmp.Option{
	cmp.Comparer(coveragePercentEqual),
	cmpopts.IgnoreFields(licensecheck.Match{}, "Start", "End"),
}

// coveragePercentEqual considers two floats the same if they are within 4
// percentage points, and both are on the same side of 90% (our threshold).
func coveragePercentEqual(a, b float64) bool {
	if (a >= 90) != (b >= 90) {
		return false
	}
	return math.Abs(a-b) <= 4
}

// NowTruncated returns time.Now() truncated to Microsecond precision.
//
// This makes it easier to work with timestamps in PostgreSQL, which have
// Microsecond precision:
//   https://www.postgresql.org/docs/9.1/datatype-datetime.html
func NowTruncated() time.Time {
	return time.Now().In(time.UTC).Truncate(time.Microsecond)
}

func DefaultModule() *internal.Module {
	fp := constructFullPath(ModulePath, Suffix)
	return AddPackage(Module(ModulePath, VersionString), UnitForPackage(fp, ModulePath, VersionString, path.Base(fp), true))
}

// Module creates a Module with the given path and version.
// The list of suffixes is used to create Units within the module.
func Module(modulePath, version string, suffixes ...string) *internal.Module {
	mi := ModuleInfo(modulePath, version)
	m := &internal.Module{
		ModuleInfo: *mi,
		Licenses:   Licenses(),
	}
	m.Units = []*internal.Unit{UnitForModuleRoot(mi)}
	for _, s := range suffixes {
		fp := constructFullPath(modulePath, s)
		lp := UnitForPackage(fp, modulePath, VersionString, path.Base(fp), m.IsRedistributable)
		if s != "" {
			AddPackage(m, lp)
		} else {
			u := UnitForPackage(lp.Path, modulePath, version, lp.Name, lp.IsRedistributable)
			m.Units[0].Documentation = u.Documentation
			m.Units[0].Name = u.Name
		}
	}
	if modulePath == stdlib.ModulePath {
		m.Units[0].Readme = nil
	}
	// Fill in license contents.
	for _, u := range m.Units {
		u.LicenseContents = m.Licenses
	}
	return m
}

func UnitForModuleRoot(m *internal.ModuleInfo) *internal.Unit {
	u := &internal.Unit{
		UnitMeta:        *UnitMeta(m.ModulePath, m.ModulePath, m.Version, "", m.IsRedistributable),
		LicenseContents: Licenses(),
	}
	u.Readme = &internal.Readme{
		Filepath: ReadmeFilePath,
		Contents: ReadmeContents,
	}
	return u
}

// UnitForPackage constructs a unit with the given module path and suffix.
//
// If modulePath is the standard library, the package path is the
// suffix, which must not be empty. Otherwise, the package path
// is the concatenation of modulePath and suffix.
//
// The package name is last component of the package path.
func UnitForPackage(path, modulePath, version, name string, isRedistributable bool) *internal.Unit {
	// Copy Doc because some tests modify it.
	doc := *Doc
	imps := Imports()
	return &internal.Unit{
		UnitMeta:        *UnitMeta(path, modulePath, version, name, isRedistributable),
		Documentation:   []*internal.Documentation{&doc},
		LicenseContents: Licenses(),
		Imports:         imps,
		NumImports:      len(imps),
	}
}

func AddPackage(m *internal.Module, pkg *internal.Unit) *internal.Module {
	if m.ModulePath != stdlib.ModulePath && !strings.HasPrefix(pkg.Path, m.ModulePath) {
		panic(fmt.Sprintf("package path %q not a prefix of module path %q",
			pkg.Path, m.ModulePath))
	}
	AddUnit(m, UnitForPackage(pkg.Path, m.ModulePath, m.Version, pkg.Name, pkg.IsRedistributable))
	minLen := len(m.ModulePath)
	if m.ModulePath == stdlib.ModulePath {
		minLen = 1
	}
	for pth := pkg.Path; len(pth) > minLen; pth = path.Dir(pth) {
		found := false
		for _, u := range m.Units {
			if u.Path == pth {
				found = true
				break
			}
		}
		if !found {
			AddUnit(m, UnitEmpty(pth, m.ModulePath, m.Version))
		}
	}
	return m
}

func PackageMeta(fullPath string) *internal.PackageMeta {
	return &internal.PackageMeta{
		Path:              fullPath,
		IsRedistributable: true,
		Name:              path.Base(fullPath),
		Synopsis:          Doc.Synopsis,
		Licenses:          LicenseMetadata(),
	}
}

func ModuleInfo(modulePath, versionString string) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath: modulePath,
		Version:    versionString,
		CommitTime: CommitTime,
		// Assume the module path is a GitHub-like repo name.
		SourceInfo:        source.NewGitHubInfo("https://"+modulePath, "", versionString),
		IsRedistributable: true,
		HasGoMod:          true,
	}
}

func DefaultVersionMap() *internal.VersionMap {
	return &internal.VersionMap{
		ModulePath:       ModulePath,
		RequestedVersion: VersionString,
		ResolvedVersion:  VersionString,
		Status:           http.StatusOK,
		GoModPath:        "",
		Error:            "",
	}
}

func AddUnit(m *internal.Module, u *internal.Unit) {
	for _, e := range m.Units {
		if e.Path == u.Path {
			panic(fmt.Sprintf("module already has path %q", e.Path))
		}
	}
	m.Units = append(m.Units, u)
}

func AddLicense(m *internal.Module, lic *licenses.License) {
	m.Licenses = append(m.Licenses, lic)
	dir := path.Dir(lic.FilePath)
	if dir == "." {
		dir = ""
	}
	for _, u := range m.Units {
		if strings.TrimPrefix(u.Path, m.ModulePath+"/") == dir {
			u.Licenses = append(u.Licenses, lic.Metadata)
			u.LicenseContents = append(u.LicenseContents, lic)
		}
	}
}

// ReplaceLicense replaces all licenses having the same file path as lic with lic.
func ReplaceLicense(m *internal.Module, lic *licenses.License) {
	replaceLicense(lic, m.Licenses)
	for _, u := range m.Units {
		for i, lm := range u.Licenses {
			if lm.FilePath == lic.FilePath {
				u.Licenses[i] = lic.Metadata
			}
		}
		replaceLicense(lic, u.LicenseContents)
	}
}

func replaceLicense(lic *licenses.License, lics []*licenses.License) {
	for i, l := range lics {
		if l.FilePath == lic.FilePath {
			lics[i] = lic
		}
	}
}

func UnitEmpty(path, modulePath, version string) *internal.Unit {
	return &internal.Unit{
		UnitMeta: *UnitMeta(path, modulePath, version, "", true),
	}
}

func UnitMeta(path, modulePath, version, name string, isRedistributable bool) *internal.UnitMeta {
	return &internal.UnitMeta{
		Path:              path,
		Name:              name,
		IsRedistributable: isRedistributable,
		Licenses:          LicenseMetadata(),
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           version,
			CommitTime:        NowTruncated(),
			IsRedistributable: isRedistributable,
			SourceInfo:        source.NewGitHubInfo("https://"+modulePath, "", version),
		},
	}
}

func constructFullPath(modulePath, suffix string) string {
	if modulePath != stdlib.ModulePath {
		return path.Join(modulePath, suffix)
	}
	return suffix
}

// Documentation returns a Documentation value for the given Go source.
// It panics if there are errors parsing or encoding the source.
func Documentation(goos, goarch, fileContents string) *internal.Documentation {
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, "sample.go", fileContents, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	docPkg := godoc.NewPackage(fset, nil)
	docPkg.AddFile(pf, true)
	src, err := docPkg.Encode(context.Background())
	if err != nil {
		panic(err)
	}
	return &internal.Documentation{
		GOOS:     goos,
		GOARCH:   goarch,
		Synopsis: fmt.Sprintf("This is a package synopsis for GOOS=%s, GOARCH=%s", goos, goarch),
		Source:   src,
	}
}

func LicenseMetadata() []*licenses.Metadata {
	return []*licenses.Metadata{
		{
			Types:    []string{LicenseType},
			FilePath: LicenseFilePath,
			OldCoverage: oldlicensecheck.Coverage{
				Percent: 100,
				Match:   []oldlicensecheck.Match{{Name: LicenseType, Type: oldlicensecheck.MIT, Percent: 100}},
			},
		},
	}
}

func Licenses() []*licenses.License {
	return []*licenses.License{
		{Metadata: LicenseMetadata()[0], Contents: []byte(`Lorem Ipsum`)},
	}
}

func Imports() []string {
	return []string{"fmt", "path/to/bar"}
}
