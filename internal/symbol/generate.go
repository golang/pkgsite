// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/build"
	"go/token"
	"go/types"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/pkgsite/internal"
)

// GenerateFeatureContexts computes the exported API for the package specified
// by pkgPath. The source code for that package is in pkgDir.
//
// It is largely adapted from
// https://go.googlesource.com/go/+/refs/heads/master/src/cmd/api/goapi.go.
func GenerateFeatureContexts(ctx context.Context, pkgPath, pkgDir string) (map[string]map[string]bool, error) {
	var contexts []*build.Context
	for _, c := range internal.BuildContexts {
		bc := &build.Context{GOOS: c.GOOS, GOARCH: c.GOARCH}
		bc.Compiler = build.Default.Compiler
		bc.ReleaseTags = build.Default.ReleaseTags
		contexts = append(contexts, bc)
	}

	var wg sync.WaitGroup
	walkers := make([]*Walker, len(internal.BuildContexts))
	for i, context := range contexts {
		i, context := i, context
		wg.Add(1)
		go func() {
			defer wg.Done()
			walkers[i] = NewWalker(context, pkgPath, pkgDir, filepath.Join(build.Default.GOROOT, "src"))
		}()
	}
	wg.Wait()
	var featureCtx = make(map[string]map[string]bool) // feature -> context name -> true
	for _, w := range walkers {
		pkg, err := w.Import(pkgPath)
		if _, nogo := err.(*build.NoGoError); nogo {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("import(%q): %v", pkgPath, err)
		}
		w.export(pkg)
		ctxName := contextName(w.context)
		for _, f := range w.Features() {
			if featureCtx[f] == nil {
				featureCtx[f] = make(map[string]bool)
			}
			featureCtx[f][ctxName] = true
		}
	}
	return featureCtx, nil
}

// FeaturesForVersion returns the set of features introduced at a given
// version.
//
// featureCtx contains all features at this version.
// prevFeatureSet contains all features in the previous version.
// newFeatures contains only features introduced at this version.
// allFeatures contains all features in the package at this version.
func FeaturesForVersion(featureCtx map[string]map[string]bool,
	prevFeatureSet map[string]bool) (newFeatures []string, featureSet map[string]bool) {
	featureSet = map[string]bool{}
	for f, cmap := range featureCtx {
		if len(cmap) == len(internal.BuildContexts) {
			if !prevFeatureSet[f] {
				newFeatures = append(newFeatures, f)
			}
			featureSet[f] = true
			continue
		}
		comma := strings.Index(f, ",")
		for cname := range cmap {
			f2 := fmt.Sprintf("%s (%s)%s", f[:comma], cname, f[comma:])
			if !prevFeatureSet[f] {
				newFeatures = append(newFeatures, f2)
			}
			featureSet[f2] = true
		}
	}
	return newFeatures, featureSet
}

// export emits the exported package features.
//
// export is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#223
// except verbose mode is removed.
func (w *Walker) export(pkg *types.Package) {
	pop := w.pushScope("pkg " + pkg.Path())
	w.current = pkg
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			w.emitObj(scope.Lookup(name))
		}
	}
	pop()
}

// Walker is the same as Walkter from
// https://go.googlesource.com/go/+/refs/heads/master/src/cmd/api/goapi.go,
// except Walker.stdPackages was renamed to Walker.packages.
type Walker struct {
	context   *build.Context
	root      string
	scope     []string
	current   *types.Package
	features  map[string]bool              // set
	imported  map[string]*types.Package    // packages already imported
	packages  []string                     // names, omitting "unsafe", internal, and vendored packages
	importMap map[string]map[string]string // importer dir -> import path -> canonical path
	importDir map[string]string            // canonical import path -> dir
}

// NewWalker is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#376,
// except w.context.Dir is set to pkgDir.
func NewWalker(context *build.Context, pkgPath, pkgDir, root string) *Walker {
	w := &Walker{
		context:  context,
		root:     root,
		features: map[string]bool{},
		imported: map[string]*types.Package{"unsafe": types.Unsafe},
	}
	w.context.Dir = pkgDir
	w.loadImports(pkgPath)
	return w
}

// listImports is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#455,
// but stdPackages was renamed to packages.
type listImports struct {
	packages  []string                     // names, omitting "unsafe", internal, and vendored packages
	importDir map[string]string            // canonical import path → directory
	importMap map[string]map[string]string // import path → canonical import path
}

// loadImports is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#483,
// except we accept pkgPath as an argument to check that pkg.ImportPath ==
// pkgPath and retry on various go list errors.
//
//
// loadImports populates w with information about the packages in the standard
// library and the packages they themselves import in w's build context.
//
// The source import path and expanded import path are identical except for vendored packages.
// For example, on return:
//
//	w.importMap["math"] = "math"
//	w.importDir["math"] = "<goroot>/src/math"
//
//	w.importMap["golang.org/x/net/route"] = "vendor/golang.org/x/net/route"
//	w.importDir["vendor/golang.org/x/net/route"] = "<goroot>/src/vendor/golang.org/x/net/route"
//
// Since the set of packages that exist depends on context, the result of
// loadImports also depends on context. However, to improve test running time
// the configuration for each environment is cached across runs.
func (w *Walker) loadImports(pkgPath string) {
	if w.context == nil {
		return // test-only Walker; does not use the import map
	}
	generateOutput := func() ([]byte, error) {
		cmd := exec.Command(goCmd(), "list", "-e", "-deps", "-json")
		cmd.Env = listEnv(w.context)
		if w.context.Dir != "" {
			cmd.Dir = w.context.Dir
		}
		return cmd.CombinedOutput()
	}

	goModDownload := func(out []byte) ([]byte, error) {
		words := strings.Fields(string(out))
		modPath := words[len(words)-1]
		cmd := exec.Command("go", "mod", "download", modPath)
		if w.context.Dir != "" {
			cmd.Dir = w.context.Dir
		}
		return cmd.CombinedOutput()
	}

	retryOrFail := func(out []byte, err error) {
		if strings.Contains(string(out), "missing go.sum entry") {
			out2, err2 := goModDownload(out)
			if err2 != nil {
				log.Fatalf("loadImports: initial error: %v\n%s \n\n error running go mod download: %v\n%s",
					err, string(out), err2, string(out2))
			}
			return
		}
		log.Fatalf("loadImports: %v\n%s", err, out)
	}

	name := contextName(w.context)
	imports, ok := listCache.Load(name)
	if !ok {
		listSem <- semToken{}
		defer func() { <-listSem }()
		out, err := generateOutput()
		if err != nil {
			retryOrFail(out, err)
		}
		if strings.HasPrefix(string(out), "go: downloading") {
			// If a module was downloaded, we will see "go: downloading
			// <module> ..." in the JSON output.
			// This causes an error in json.NewDecoder below, so run
			// generateOutput again to avoid that error.
			out, err = generateOutput()
			if err != nil {
				retryOrFail(out, err)
			}
		}

		var packages []string
		importMap := make(map[string]map[string]string)
		importDir := make(map[string]string)
		dec := json.NewDecoder(bytes.NewReader(out))
		for {
			var pkg struct {
				ImportPath, Dir string
				ImportMap       map[string]string
				Standard        bool
			}
			err := dec.Decode(&pkg)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("loadImports: go list: invalid output: %v", err)
			}
			// - Package "unsafe" contains special signatures requiring
			//   extra care when printing them - ignore since it is not
			//   going to change w/o a language change.
			// - Internal and vendored packages do not contribute to our
			//   API surface. (If we are running within the "std" module,
			//   vendored dependencies appear as themselves instead of
			//   their "vendor/" standard-library copies.)
			// - 'go list std' does not include commands, which cannot be
			//   imported anyway.
			if ip := pkg.ImportPath; pkg.ImportPath == pkgPath ||
				(pkg.Standard && ip != "unsafe" && !strings.HasPrefix(ip, "vendor/") && !internalPkg.MatchString(ip)) {
				packages = append(packages, ip)
			}
			importDir[pkg.ImportPath] = pkg.Dir
			if len(pkg.ImportMap) > 0 {
				importMap[pkg.Dir] = make(map[string]string, len(pkg.ImportMap))
			}
			for k, v := range pkg.ImportMap {
				importMap[pkg.Dir][k] = v
			}
		}
		sort.Strings(packages)
		imports = listImports{
			packages:  packages,
			importMap: importMap,
			importDir: importDir,
		}
		imports, _ = listCache.LoadOrStore(name, imports)
	}
	li := imports.(listImports)
	w.packages = li.packages
	w.importDir = li.importDir
	w.importMap = li.importMap
}

// emitStructType is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#931,
// except we also check if a field is Embedded. If so, we ignore that field.
func (w *Walker) emitStructType(name string, typ *types.Struct) {
	typeStruct := fmt.Sprintf("type %s struct", name)
	w.emitf(typeStruct)
	defer w.pushScope(typeStruct)()
	for i := 0; i < typ.NumFields(); i++ {
		f := typ.Field(i)
		if f.Embedded() {
			continue
		}
		if !f.Exported() {
			continue
		}
		typ := f.Type()
		if f.Anonymous() {
			w.emitf("embedded %s", w.typeString(typ))
			continue
		}
		w.emitf("%s %s", f.Name(), w.typeString(typ))
	}
}

// emitIfaceType is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#931,
// except we don't check for unexported methods.
func (w *Walker) emitIfaceType(name string, typ *types.Interface) {
	typeInterface := fmt.Sprintf("type " + name + " interface")
	w.emitf(typeInterface)
	pop := w.pushScope(typeInterface)

	var methodNames []string
	for i := 0; i < typ.NumExplicitMethods(); i++ {
		m := typ.ExplicitMethod(i)
		if m.Exported() {
			methodNames = append(methodNames, m.Name())
			w.emitf("%s%s", m.Name(), w.signatureString(m.Type().(*types.Signature)))
		}
	}
	pop()

	sort.Strings(methodNames)
}

// emitf is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#997,
// except verbose mode is removed.
func (w *Walker) emitf(format string, args ...interface{}) {
	f := strings.Join(w.scope, ", ") + ", " + fmt.Sprintf(format, args...)
	if strings.Contains(f, "\n") {
		panic("feature contains newlines: " + f)
	}
	if _, dup := w.features[f]; dup {
		panic("duplicate feature inserted: " + f)
	}
	w.features[f] = true
}

// goCmd is the same as
// https://go.googlesource.com/go/+/refs/tags/go1.16.6/src/cmd/api/goapi.go#31,
// except support for Windows is removed.
func goCmd() string {
	path := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "go"
}
