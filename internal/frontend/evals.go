// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The evaluations page, which displays signals for
// package and module quality.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"math"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/version"
)

type evalType struct {
	Label       string // displayed on the page
	Description string // displayed from "?" icon
	MaxScore    int    // number of bars
}

var (
	licenseEval = &evalType{
		Label: "License for this package or module",
		Description: `The module license.
0: no license
1: non-redistributable license
2: redistributable license`,
		MaxScore: 2,
	}

	moduleVersionEval = &evalType{
		Label: "Module version",
		Description: `Version of this module.
0: untagged
1: tagged, unstable (v0)
2: tagged, stable (v1 or higher)`,
		MaxScore: 2,
	}

	docCoverageEval = &evalType{
		Label: "Documentation coverage",
		Description: `Score for documentation on package and exported symbols.
The score is 10% whether the package has a comment, 90% the fraction of exported
symbols that are commented. The format of the comments doesn't matter.
0: no comments
1: 1% - 20%
2: 21% - 40%
3: 41% - 60%
4: 61% - 80%
5: 81% - 100%`,
		MaxScore: 5,
	}
)

type eval struct {
	Type  *evalType
	Value string // displayed on the page
	Score int    // number of colored bars
}

type evalsDetails struct {
	Evals []eval
}

func fetchEvalsDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) (*evalsDetails, error) {
	u, err := ds.GetUnit(ctx, um, internal.WithLicenses|internal.WithDocsSource, internal.BuildContext{})
	if err != nil {
		return nil, err
	}

	licEval := eval{Type: licenseEval}
	switch {
	case len(u.LicenseContents) == 0:
		licEval.Score = 0
		licEval.Value = "no license"
	case !u.IsRedistributable:
		licEval.Score = 1
		licEval.Value = "non-redistributable license"
	default:
		licEval.Score = 2
		licEval.Value = "redistributable license"
	}

	modEval := eval{Type: moduleVersionEval}
	versionType, err := version.ParseType(um.Version)
	if err != nil {
		return nil, err
	}
	switch {
	case version.IsPseudo(um.Version) || !semver.IsValid(um.Version):
		modEval.Score = 0
		modEval.Value = "untagged"
	case semver.Major(um.Version) == "v0" || versionType == version.TypePrerelease:
		modEval.Score = 1
		modEval.Value = "tagged, unstable"
	default:
		modEval.Score = 2
		modEval.Value = "tagged, stable"
	}

	var docPkg *godoc.Package
	if len(u.Documentation) > 0 && len(u.Documentation[0].Source) > 0 {
		var err error
		docPkg, err = godoc.DecodePackage(u.Documentation[0].Source)
		if err != nil {
			return nil, fmt.Errorf("decoding package: %w", err)
		}
	}
	docEval := evalDocCoverage(summarizeDocumentation(docPkg))

	return &evalsDetails{
		Evals: []eval{licEval, modEval, docEval},
	}, nil
}

// evalDocCoverage converts a docSummary into an evaluation.
// The fraction of exported symbols documented is combined
// with the presence of a package comment to produce a raw Score out of 100.
// The number of bars (docEval.Score) divides this range into groups of 20.
// Thus a 5% score is 1 bar, an 82% score is 5 bars, and so on.
//
// The presence of a package doc is worth packageDocFraction, and the fraction
// of documented symbols counts for the rest. For example, if packageDocFraction = 10%,
// then the raw score is:
//
//	10% * (1 if package doc else 0) + 90% * (fraction of doc symbols)
func evalDocCoverage(summary docSummary) eval {
	const packageDocFraction = 0.10

	docEval := eval{Type: docCoverageEval}
	if !summary.packageHasDoc && summary.numHaveDoc == 0 {
		docEval.Score = 0
		docEval.Value = "no comments"
		return docEval
	}

	var rawScore float64
	if summary.packageHasDoc {
		rawScore += packageDocFraction
	}
	f := 1.0
	if summary.numExportedSymbols > 0 {
		f = float64(summary.numHaveDoc) / float64(summary.numExportedSymbols)
	}
	rawScore += (1 - packageDocFraction) * f

	// The score (number of bars) is basically the percentage divided by 20.
	pct := int(math.Round(rawScore * 100))
	if pct == 0 {
		// We know rawScore > 0, so don't use a zero here. Reserve that for
		// when there is really no documentation at all.
		docEval.Score = 1
		// Show percentage to one decimal; "0%" would be confusing.
		// (Although we might still get "0.0%" sometimes.)
		docEval.Value = fmt.Sprintf("%.1f%%", rawScore*100)
	} else {
		// Map a percentage from 1 to 100 to a score from 1 to 5,
		// with 1-20 => 1, 21-40 => 2, etc.
		docEval.Score = (pct-1)/20 + 1
		docEval.Value = fmt.Sprintf("%d%%", pct)
	}
	return docEval
}

const (
	day   = 24 * time.Hour
	month = 31 * day
	year  = 365 * day
)

// ageString formats a time.Duration into a human-readable age string.
func ageString(dur time.Duration) string {
	pluralize := func(n time.Duration, s string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", s)
		}
		return fmt.Sprintf("%d %ss", n, s)
	}

	if dur < day {
		return "less than a day"
	}
	if dur < month {
		return pluralize(dur/day, "day")
	}
	if dur < year {
		monthStr := pluralize(dur/month, "month")
		days := (dur % month) / day
		if days == 0 {
			return monthStr
		}
		return monthStr + ", " + pluralize(days, "day")
	}

	yearStr := pluralize(dur/year, "year")
	months := (dur % year) / month
	if months == 0 {
		return yearStr
	}
	return yearStr + ", " + pluralize(months, "month")
}

// docSummary is a summary of a package's documentation, intended
// for creating an evaluation.
type docSummary struct {
	packageHasDoc      bool
	numExportedSymbols int // that need documentation
	numHaveDoc         int // number of exported symbols that need doc and have it
}

// summarizeDocumentation produces a docSummary for the given package.
func summarizeDocumentation(docPkg *godoc.Package) docSummary {
	var summary docSummary

	if docPkg == nil || len(docPkg.Files) == 0 {
		return summary
	}

	var files []*ast.File
	for _, f := range docPkg.Files {
		if f == nil || f.AST == nil {
			continue
		}
		if strings.HasSuffix(f.Name, "_test.go") {
			continue
		}
		files = append(files, f.AST)
	}

	if len(files) == 0 {
		return summary
	}

	// We don't insist that main packages have doc.
	// TODO(jba): consider checking the documentation of main packages.
	// They should at least have a package doc.
	// On the other hand, many main packages have an extensive README.md but
	// no package doc.
	pkgName := files[0].Name.Name
	if pkgName == "main" {
		return summary
	}

	for _, file := range files {
		if file.Doc != nil {
			summary.packageHasDoc = true
		}
	}

	CollectSymbols(files, func(_ *ast.Ident, has bool) {
		summary.numExportedSymbols++
		if has {
			summary.numHaveDoc++
		}
	})

	return summary
}

// collectInterfaceMethods walks files looking for top-level exported interface
// declarations. For every exported method in those interfaces (including methods
// embedded from other interfaces) that has documentation, it records a mapping
// from the method's name to its signature (e.g., "() string").
// This map is formatted identically to conventionalMethods.
func collectInterfaceMethods(files []*ast.File) map[interfaceMethod]bool {
	// First pass: index all top-level interface declarations in the package by their
	// type name (both exported and unexported). We need to index unexported interfaces
	// as well because an exported interface may embed an unexported interface from the
	// same package, and we must be able to look up its AST to collect those embedded methods.
	ifaceMap := make(map[string]*ast.InterfaceType)
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if t, ok := ts.Type.(*ast.InterfaceType); ok {
					ifaceMap[ts.Name.Name] = t
				}
			}
		}
	}

	methods := make(map[interfaceMethod]bool)

	// collect visits all methods defined in itype and adds any exported,
	// documented methods to the methods map. When it encounters an embedded interface
	// (or type constraint), it resolves the interface name in ifaceMap and recursively
	// collects from the embedded interface. The visited map tracks interfaces currently
	// being traversed to prevent infinite recursion in case of cyclic or repeated embedding.
	var collect func(itype *ast.InterfaceType, visited map[string]bool)
	collect = func(itype *ast.InterfaceType, visited map[string]bool) {
		if itype == nil || itype.Methods == nil {
			return
		}
		for _, field := range itype.Methods.List {
			if len(field.Names) == 0 { // embedded interface or union
				embeddedName := identName(field.Type)
				if embeddedName != "" && !visited[embeddedName] {
					if ei, ok := ifaceMap[embeddedName]; ok {
						visited[embeddedName] = true
						collect(ei, visited)
					}
				}
				continue
			}
			ftype, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			if !hasComment(field.Doc) && !hasComment(field.Comment) {
				continue
			}
			// There should only be one name, but just to play it safe, loop.
			for _, name := range field.Names {
				if name.IsExported() {
					methods[interfaceMethod{name.Name, sigString(ftype)}] = true
				}
			}
		}
	}

	// Second pass: iterate over all collected interfaces and initiate method collection
	// only for top-level exported interfaces. Methods from unexported interfaces will
	// only be included if they were embedded within one of these exported interfaces.
	visited := make(map[string]bool)
	for name, itype := range ifaceMap {
		if ast.IsExported(name) && !visited[name] {
			visited[name] = true
			collect(itype, visited)
		}
	}
	return methods
}

// CollectSymbols visits files looking for exported symbols that need
// documentation. It calls add(id, true) for a symbol if it has documentation,
// and add(id, false) if it does not, where id is the *ast.Ident of the symbol.
//
// This signature is overkill for production use, where we just want to count.
// But it's useful for tests, to compare sets of symbols.
//
// This function accepts various forms of documentation, such as trailing line
// comments or group-level comments, to capture a broad signal of documented
// API surface. It does not enforce standard Go documentation formatting
// (e.g., "Name does...").
func CollectSymbols(files []*ast.File, add func(id *ast.Ident, has bool)) {
	for _, file := range files {
		if file != nil && file.Doc != nil && dochtml.IsDeprecated(file.Doc.Text()) {
			return
		}
	}

	ifaceMethods := collectInterfaceMethods(files)

	// specDoc finds the doc comment for a spec. Typically this will be doc itself,
	// but if absent:
	// - for consts and vars, we use the comment on the enclosing GenDecl (d.Doc)
	// - for types, we only use the GenDecl comment if this is a single-spec declaration
	//   (e.g. `type T struct{}`),
	// If both comments are absent, comment is a trailing line comment on the same
	// line, which we generously accept.
	specDoc := func(doc, comment *ast.CommentGroup, d *ast.GenDecl) *ast.CommentGroup {
		if doc != nil {
			return doc
		}
		if d.Doc != nil && (d.Tok != token.TYPE || len(d.Specs) == 1) {
			return d.Doc
		}
		return comment
	}

	// Walk the top-level declarations of every file.
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				// If a method, both the method and the receiver must be exported.
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv != nil {
					recvName := recvTypeName(d.Recv)
					if !ast.IsExported(recvName) {
						continue
					}
					m := interfaceMethod{name: d.Name.Name, signature: sigString(d.Type)}
					if conventionalMethods[m] || ifaceMethods[m] {
						continue
					}
				}
				add(d.Name, hasComment(d.Doc))
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							doc := specDoc(s.Doc, s.Comment, d)
							add(s.Name, hasComment(doc))
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								doc := specDoc(s.Doc, s.Comment, d)
								add(name, hasComment(doc))
							}
						}
					}
				}
			}
		}
	}
}

// recvTypeName extracts the type name of the receiver from a receiver field list,
// handling pointer receivers and generic type instantiations (e.g., *T or T[P]).
// The receiver type is the first one in the list.
// It returns "" if it cannot find a receiver type.
func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return identName(recv.List[0].Type)
}

// identName extracts the name of an identifier from an expression,
// handling pointer expressions (*T) and generic type instantiations (T[P] or T[P1, P2]).
func identName(expr ast.Expr) string {
	if ptr, ok := expr.(*ast.StarExpr); ok {
		expr = ptr.X
	}
	switch x := expr.(type) {
	case *ast.IndexExpr:
		expr = x.X
	case *ast.IndexListExpr:
		expr = x.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// hasComment reports whether the comment group actually contains a non-empty comment.
func hasComment(doc *ast.CommentGroup) bool {
	return doc != nil && strings.TrimSpace(doc.Text()) != ""
}

// An interfaceMethod is a method from an interface.
type interfaceMethod struct {
	name      string
	signature string
}

// conventionalMethods maps common, standard method names like String and Error
// to their signatures. These methods are often undocumented, and that's fine.
var conventionalMethods = map[interfaceMethod]bool{
	{"String", "() string"}:               true, // fmt.Stringer
	{"Error", "() string"}:                true, // error
	{"Unwrap", "() error"}:                true, // for errors.Unwrap
	{"Len", "() int"}:                     true, // sort.Interface
	{"Less", "(int, int) bool"}:           true, // sort.Interface
	{"Swap", "(int, int)"}:                true, // sort.Interface
	{"Read", "([]byte) (int, error)"}:     true, // io.Reader
	{"Close", "() error"}:                 true, // io.ReadCloser
	{"Write", "([]byte) (int, error)"}:    true, // io.Writer
	{"MarshalJSON", "() ([]byte, error)"}: true, // json.Marshaler
	{"UnmarshalJSON", "([]byte) error"}:   true, // json.Unmarshaler
}

// sigString returns a string representation of a function signature
// without parameter or return names (e.g., "(int, int) bool").
func sigString(ft *ast.FuncType) string {
	if ft == nil {
		return ""
	}
	paramTypes := fieldListTypes(ft.Params)
	resTypes := fieldListTypes(ft.Results)

	var buf strings.Builder
	buf.WriteString("(")
	buf.WriteString(strings.Join(paramTypes, ", "))
	buf.WriteString(")")

	if len(resTypes) == 1 {
		buf.WriteString(" ")
		buf.WriteString(resTypes[0])
	} else if len(resTypes) > 1 {
		buf.WriteString(" (")
		buf.WriteString(strings.Join(resTypes, ", "))
		buf.WriteString(")")
	}
	return buf.String()
}

func fieldListTypes(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var types []string
	for _, field := range fl.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		tstr := nodeString(field.Type)
		for i := 0; i < n; i++ {
			types = append(types, tstr)
		}
	}
	return types
}

// nodeString returns a string for node.
func nodeString(node ast.Node) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, token.NewFileSet(), node)
	return buf.String()
}
