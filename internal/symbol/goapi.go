// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// DO NOT EDIT.
// All the code in this file is manually copied from
// https://go.googlesource.com/go/+/refs/heads/master/src/cmd/api/goapi.go.
// If an identifier needs to be edited, move it to internal/symbol/generate.go
// before editing.

package symbol

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var (
	fset        = token.NewFileSet()
	internalPkg = regexp.MustCompile(`(^|/)internal($|/)`)
)

func (w *Walker) Features() (fs []string) {
	for f := range w.features {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	return
}

var parsedFileCache = make(map[string]*ast.File)

func (w *Walker) parseFile(dir, file string) (*ast.File, error) {
	filename := filepath.Join(dir, file)
	if f := parsedFileCache[filename]; f != nil {
		return f, nil
	}
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}
	parsedFileCache[filename] = f
	return f, nil
}

// Disable before debugging non-obvious errors from the type-checker.

const usePkgCache = true

var (
	pkgCache = map[string]*types.Package{} // map tagKey to package
	pkgTags  = map[string][]string{}       // map import dir to list of relevant tags
)

// tagKey returns the tag-based key to use in the pkgCache.
// It is a comma-separated string; the first part is dir, the rest tags.
// The satisfied tags are derived from context but only those that
// matter (the ones listed in the tags argument plus GOOS and GOARCH) are used.
// The tags list, which came from go/build's Package.AllTags,
// is known to be sorted.
func tagKey(dir string, context *build.Context, tags []string) string {
	ctags := map[string]bool{
		context.GOOS:   true,
		context.GOARCH: true,
	}
	if context.CgoEnabled {
		ctags["cgo"] = true
	}
	for _, tag := range context.BuildTags {
		ctags[tag] = true
	}
	// TODO: ReleaseTags (need to load default)
	key := dir
	// explicit on GOOS and GOARCH as global cache will use "all" cached packages for
	// an indirect imported package. See https://github.com/golang/go/issues/21181
	// for more detail.
	tags = append(tags, context.GOOS, context.GOARCH)
	sort.Strings(tags)
	for _, tag := range tags {
		if ctags[tag] {
			key += "," + tag
			ctags[tag] = false
		}
	}
	return key
}

var listCache sync.Map // map[string]listImports, keyed by contextName

// listSem is a semaphore restricting concurrent invocations of 'go list'.
var listSem = make(chan semToken, runtime.GOMAXPROCS(0))

type semToken struct{}

// listEnv returns the process environment to use when invoking 'go list' for
// the given context.
func listEnv(c *build.Context) []string {
	if c == nil {
		return os.Environ()
	}
	environ := append(os.Environ(),
		"GOOS="+c.GOOS,
		"GOARCH="+c.GOARCH)
	if c.CgoEnabled {
		environ = append(environ, "CGO_ENABLED=1")
	} else {
		environ = append(environ, "CGO_ENABLED=0")
	}
	return environ
}

// Importing is a sentinel taking the place in Walker.imported
// for a package that is in the process of being imported.
var importing types.Package

func (w *Walker) Import(name string) (*types.Package, error) {
	return w.ImportFrom(name, "", 0)
}

func (w *Walker) ImportFrom(fromPath, fromDir string, mode types.ImportMode) (*types.Package, error) {
	name := fromPath
	if canonical, ok := w.importMap[fromDir][fromPath]; ok {
		name = canonical
	}
	pkg := w.imported[name]
	if pkg != nil {
		if pkg == &importing {
			log.Fatalf("cycle importing package %q", name)
		}
		return pkg, nil
	}
	w.imported[name] = &importing
	// Determine package files.
	dir := w.importDir[name]
	if dir == "" {
		dir = filepath.Join(w.root, filepath.FromSlash(name))
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("no source in tree for import %q (from import %s in %s): %v", name, fromPath, fromDir, err)
	}
	context := w.context
	if context == nil {
		context = &build.Default
	}

	// Look in cache.
	// If we've already done an import with the same set
	// of relevant tags, reuse the result.
	var key string
	if usePkgCache {
		if tags, ok := pkgTags[dir]; ok {
			key = tagKey(dir, context, tags)
			if pkg := pkgCache[key]; pkg != nil {
				w.imported[name] = pkg
				return pkg, nil
			}
		}
	}
	info, err := context.ImportDir(dir, 0)
	if err != nil {
		if _, nogo := err.(*build.NoGoError); nogo {
			return nil, err
		}
		log.Fatalf("pkg %q, dir %q: ScanDir: %v", name, dir, err)
	}
	// Save tags list first time we see a directory.
	if usePkgCache {
		if _, ok := pkgTags[dir]; !ok {
			pkgTags[dir] = info.AllTags
			key = tagKey(dir, context, info.AllTags)
		}
	}
	filenames := append(append([]string{}, info.GoFiles...), info.CgoFiles...)
	// Parse package files.
	var files []*ast.File
	for _, file := range filenames {
		f, err := w.parseFile(dir, file)
		if err != nil {
			log.Fatalf("error parsing package %s: %s", name, err)
		}
		files = append(files, f)
	}
	// Type-check package files.
	conf := types.Config{
		IgnoreFuncBodies: true,
		FakeImportC:      true,
		Importer:         w,
	}
	pkg, err = conf.Check(name, fset, files, nil)
	if err != nil {
		ctxt := "<no context>"
		if w.context != nil {
			ctxt = fmt.Sprintf("%s-%s", w.context.GOOS, w.context.GOARCH)
		}
		return nil, fmt.Errorf("error typechecking package %s: %s (%s)", name, err, ctxt)
	}
	if usePkgCache {
		pkgCache[key] = pkg
	}
	w.imported[name] = pkg
	return pkg, nil
}

// pushScope enters a new scope (walking a package, type, node, etc)
// and returns a function that will leave the scope (with sanity checking
// for mismatched pushes & pops)
func (w *Walker) pushScope(name string) (popFunc func()) {
	w.scope = append(w.scope, name)
	return func() {
		if len(w.scope) == 0 {
			log.Fatalf("attempt to leave scope %q with empty scope list", name)
		}
		if w.scope[len(w.scope)-1] != name {
			log.Fatalf("attempt to leave scope %q, but scope is currently %#v", name, w.scope)
		}
		w.scope = w.scope[:len(w.scope)-1]
	}
}

func sortedMethodNames(typ *types.Interface) []string {
	n := typ.NumMethods()
	list := make([]string, n)
	for i := range list {
		list[i] = typ.Method(i).Name()
	}
	sort.Strings(list)
	return list
}

func (w *Walker) writeType(buf *bytes.Buffer, typ types.Type) {
	switch typ := typ.(type) {
	case *types.Basic:
		s := typ.Name()
		switch typ.Kind() {
		case types.UnsafePointer:
			s = "unsafe.Pointer"
		case types.UntypedBool:
			s = "ideal-bool"
		case types.UntypedInt:
			s = "ideal-int"
		case types.UntypedRune:
			// "ideal-char" for compatibility with old tool
			// TODO(gri) change to "ideal-rune"
			s = "ideal-char"
		case types.UntypedFloat:
			s = "ideal-float"
		case types.UntypedComplex:
			s = "ideal-complex"
		case types.UntypedString:
			s = "ideal-string"
		case types.UntypedNil:
			panic("should never see untyped nil type")
		default:
			switch s {
			case "byte":
				s = "uint8"
			case "rune":
				s = "int32"
			}
		}
		buf.WriteString(s)
	case *types.Array:
		fmt.Fprintf(buf, "[%d]", typ.Len())
		w.writeType(buf, typ.Elem())
	case *types.Slice:
		buf.WriteString("[]")
		w.writeType(buf, typ.Elem())
	case *types.Struct:
		buf.WriteString("struct")
	case *types.Pointer:
		buf.WriteByte('*')
		w.writeType(buf, typ.Elem())
	case *types.Tuple:
		panic("should never see a tuple type")
	case *types.Signature:
		buf.WriteString("func")
		w.writeSignature(buf, typ)
	case *types.Interface:
		buf.WriteString("interface{")
		if typ.NumMethods() > 0 {
			buf.WriteByte(' ')
			buf.WriteString(strings.Join(sortedMethodNames(typ), ", "))
			buf.WriteByte(' ')
		}
		buf.WriteString("}")
	case *types.Map:
		buf.WriteString("map[")
		w.writeType(buf, typ.Key())
		buf.WriteByte(']')
		w.writeType(buf, typ.Elem())
	case *types.Chan:
		var s string
		switch typ.Dir() {
		case types.SendOnly:
			s = "chan<- "
		case types.RecvOnly:
			s = "<-chan "
		case types.SendRecv:
			s = "chan "
		default:
			panic("unreachable")
		}
		buf.WriteString(s)
		w.writeType(buf, typ.Elem())
	case *types.Named:
		obj := typ.Obj()
		pkg := obj.Pkg()
		if pkg != nil && pkg != w.current {
			buf.WriteString(pkg.Name())
			buf.WriteByte('.')
		}
		buf.WriteString(typ.Obj().Name())
	default:
		panic(fmt.Sprintf("unknown type %T", typ))
	}
}

func (w *Walker) writeSignature(buf *bytes.Buffer, sig *types.Signature) {
	w.writeParams(buf, sig.Params(), sig.Variadic())
	switch res := sig.Results(); res.Len() {
	case 0:
		// nothing to do
	case 1:
		buf.WriteByte(' ')
		w.writeType(buf, res.At(0).Type())
	default:
		buf.WriteByte(' ')
		w.writeParams(buf, res, false)
	}
}

func (w *Walker) writeParams(buf *bytes.Buffer, t *types.Tuple, variadic bool) {
	buf.WriteByte('(')
	for i, n := 0, t.Len(); i < n; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		typ := t.At(i).Type()
		if variadic && i+1 == n {
			buf.WriteString("...")
			typ = typ.(*types.Slice).Elem()
		}
		w.writeType(buf, typ)
	}
	buf.WriteByte(')')
}

func (w *Walker) typeString(typ types.Type) string {
	var buf bytes.Buffer
	w.writeType(&buf, typ)
	return buf.String()
}

func (w *Walker) signatureString(sig *types.Signature) string {
	var buf bytes.Buffer
	w.writeSignature(&buf, sig)
	return buf.String()
}

func (w *Walker) emitObj(obj types.Object) {
	switch obj := obj.(type) {
	case *types.Const:
		w.emitf("const %s %s", obj.Name(), w.typeString(obj.Type()))
		x := obj.Val()
		short := x.String()
		exact := x.ExactString()
		if short == exact {
			w.emitf("const %s = %s", obj.Name(), short)
		} else {
			w.emitf("const %s = %s  // %s", obj.Name(), short, exact)
		}
	case *types.Var:
		w.emitf("var %s %s", obj.Name(), w.typeString(obj.Type()))
	case *types.TypeName:
		w.emitType(obj)
	case *types.Func:
		w.emitFunc(obj)
	default:
		panic("unknown object: " + obj.String())
	}
}

func (w *Walker) emitType(obj *types.TypeName) {
	name := obj.Name()
	typ := obj.Type()
	if obj.IsAlias() {
		w.emitf("type %s = %s", name, w.typeString(typ))
		return
	}
	switch typ := typ.Underlying().(type) {
	case *types.Struct:
		w.emitStructType(name, typ)
	case *types.Interface:
		w.emitIfaceType(name, typ)
		return // methods are handled by emitIfaceType
	default:
		w.emitf("type %s %s", name, w.typeString(typ.Underlying()))
	}
	// emit methods with value receiver
	var methodNames map[string]bool
	vset := types.NewMethodSet(typ)
	for i, n := 0, vset.Len(); i < n; i++ {
		m := vset.At(i)
		if m.Obj().Exported() {
			// Do not emit methods promoted from embedded fields.
			if len(m.Index()) == 1 {
				w.emitMethod(m)
			}
			if methodNames == nil {
				methodNames = make(map[string]bool)
			}
			methodNames[m.Obj().Name()] = true
		}
	}
	// emit methods with pointer receiver; exclude
	// methods that we have emitted already
	// (the method set of *T includes the methods of T)
	pset := types.NewMethodSet(types.NewPointer(typ))
	for i, n := 0, pset.Len(); i < n; i++ {
		m := pset.At(i)
		if m.Obj().Exported() && !methodNames[m.Obj().Name()] {
			// Do not emit methods promoted from embedded fields.
			if len(m.Index()) == 1 {
				w.emitMethod(m)
			}
		}
	}
}

func (w *Walker) emitFunc(f *types.Func) {
	sig := f.Type().(*types.Signature)
	if sig.Recv() != nil {
		panic("method considered a regular function: " + f.String())
	}
	w.emitf("func %s%s", f.Name(), w.signatureString(sig))
}

func (w *Walker) emitMethod(m *types.Selection) {
	sig := m.Type().(*types.Signature)
	recv := sig.Recv().Type()
	// report exported methods with unexported receiver base type
	if true {
		base := recv
		if p, _ := recv.(*types.Pointer); p != nil {
			base = p.Elem()
		}
		if obj := base.(*types.Named).Obj(); !obj.Exported() {
			log.Fatalf("exported method with unexported receiver base type: %s", m)
		}
	}
	w.emitf("method (%s) %s%s", w.typeString(recv), m.Obj().Name(), w.signatureString(sig))
}

func contextName(c *build.Context) string {
	s := c.GOOS + "-" + c.GOARCH
	if c.CgoEnabled {
		s += "-cgo"
	}
	if c.Dir != "" {
		s += fmt.Sprintf(" [%s]", c.Dir)
	}
	return s
}
