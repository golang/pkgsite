// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The evaldoc command is an HTTP server that takes a module_path@version or a local directory path,
// loads the module, and serves:
//   - Root (/): A list of all import paths in the module with counts of symbols that have and need
//     documentation.
//   - /<importPath>: A list of files in that package directory with counts of symbols that have
//     and need documentation.
//   - /<importPath>/<filename>: The contents of that file with symbol documentation highlighting.
//
// It can be used to better understand the "documentation coverage" score
// on a package's evaluations page (pkg.go.dev/IMPORT/PATH?tab=evals).
// Run it on your module to get a list of symbols which need documentation,
// and whether they have documentation.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/proxy"
)

var (
	httpAddr = flag.String("http", ":0", "address to listen on")
	proxyURL = flag.String("proxy", "", "module proxy URL (defaults to GOPROXY or https://proxy.golang.org)")
)

// highlight is a byte range [start, end) in a source file corresponding to a symbol
// identifier, and whether that symbol has documentation.
type highlight struct {
	start int
	end   int
	has   bool
}

// importPathItem contains documentation coverage statistics for a package import
// path.
// Fields are exported because this struct appears in a template argument.
type importPathItem struct {
	ImportPath       string
	NumHave, NumNeed int // number of symbols that have/need doc
	NeedPct          int // percentage of symbols that need doc
}

// fileItem contains documentation coverage statistics for an individual source file.
// Fields are exported because this struct appears in a template argument.
type fileItem struct {
	Filename         string
	NumHave, NumNeed int // number of symbols that have/need doc
	NeedPct          int // percentage of symbols that need doc
}

// server holds state and pre-rendered HTML data for serving the evaldoc web UI.
type server struct {
	modulePath      string
	resolvedVersion string
	importPaths     []importPathItem
	dirFiles        map[string][]fileItem    // importPath -> list of file items
	fileHTML        map[string]template.HTML // importPath + "/" + filename -> HTML content
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] (module_path@version | local_dir_path)\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "local_dir_path must start with one of: / . ~")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	arg := flag.Arg(0)
	modulePath, resolvedVersion, contentDir, err := getContentDir(arg)
	if err != nil {
		log.Fatal(err)
	}

	s, err := newServer(modulePath, resolvedVersion, contentDir)
	if err != nil {
		log.Fatalf("newServer: %v", err)
	}

	addr := *httpAddr
	if addr == "" {
		addr = ":0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Serving %s@%s on %s", modulePath, resolvedVersion, ln.Addr())
	if err := http.Serve(ln, s); err != nil {
		log.Fatal(err)
	}
}

// resolveLocalPath expands a leading "~" to the user's home directory and returns
// the absolute path of arg.
func resolveLocalPath(arg string) (string, error) {
	if strings.HasPrefix(arg, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("arg is local path, but cannot get home directory: %v", err)
		}
		arg = filepath.Join(home, arg[1:])
	}
	return filepath.Abs(arg)
}

// getContentDir uses arg to find a module, and returns an fs.FS whose root is the
// content directory of that module (the directory containing the go.mod file).
// It also returns the module path and the version, with "latest" resolved to a specific version.
func getContentDir(arg string) (modulePath, resolvedVersion string, contentDir fs.FS, err error) {
	if len(arg) == 0 {
		return "", "", nil, errors.New("empty argument")
	}
	if arg[0] == '/' || arg[0] == '.' || arg[0] == '~' {
		localPath, err := resolveLocalPath(arg)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to resolve path %s: %w", arg, err)
		}
		fi, err := os.Stat(localPath)
		if err != nil || !fi.IsDir() {
			return "", "", nil, fmt.Errorf("%s is not a directory", localPath)
		}
		modBytes, err := os.ReadFile(filepath.Join(localPath, "go.mod"))
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to read go.mod in %s: %w", localPath, err)
		}
		modulePath = modfile.ModulePath(modBytes)
		if modulePath == "" {
			return "", "", nil, fmt.Errorf("go.mod in %s contains no module path", localPath)
		}
		return modulePath, "local", os.DirFS(localPath), nil
	}

	var reqVer string
	var found bool
	modulePath, reqVer, found = strings.Cut(arg, "@")
	if !found || reqVer == "" {
		reqVer = "latest"
	}

	pURL := *proxyURL
	if pURL == "" {
		pURL = os.Getenv("GOPROXY")
	}
	if pURL == "off" {
		return "", "", nil, errors.New("GOPROXY is off")
	}
	if pURL == "" {
		pURL = "https://proxy.golang.org"
	}
	// Take the first URL from the GOPROXY list.
	if idx := strings.IndexAny(pURL, ",|"); idx != -1 {
		pURL = pURL[:idx]
	}

	ctx := context.Background()
	proxyClient, err := proxy.New(pURL, http.DefaultTransport)
	if err != nil {
		return "", "", nil, fmt.Errorf("proxy.New(%q): %w", pURL, err)
	}
	proxyClient = proxyClient.WithFetchDisabled()

	verInfo, err := proxyClient.Info(ctx, modulePath, reqVer)
	if err != nil {
		return "", "", nil, fmt.Errorf("proxyClient.Info(%q, %q): %w", modulePath, reqVer, err)
	}
	resolvedVersion = verInfo.Version

	zipReader, err := proxyClient.Zip(ctx, modulePath, resolvedVersion)
	if err != nil {
		return "", "", nil, fmt.Errorf("proxyClient.Zip(%q, %q): %w", modulePath, resolvedVersion, err)
	}

	contentDir, err = fs.Sub(zipReader, modulePath+"@"+resolvedVersion)
	if err != nil {
		return "", "", nil, fmt.Errorf("fs.Sub: %w", err)
	}

	return modulePath, resolvedVersion, contentDir, nil
}

// packageDirs walks contentDir to find directories corresponding to valid import paths,
// and returns their relative paths.
func packageDirs(contentDir fs.FS) ([]string, error) {
	var dirs []string
	err := fs.WalkDir(contentDir, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// Skip directory trees under internal, vendor and testdata.
		name := d.Name()
		if name == "internal" || name == "vendor" || name == "testdata" {
			return fs.SkipDir
		}
		// Skip directory trees if the name starts with a ".", other than "." itself.
		if len(name) >= 2 && name[0] == '.' {
			return fs.SkipDir
		}
		dirs = append(dirs, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

// newServer parses and analyzes all non-internal Go package files in contentDir,
// calculates symbol documentation statistics, pre-renders HTML views, and returns
// a server for modulePath and resolvedVersion.
func newServer(modulePath, resolvedVersion string, contentDir fs.FS) (*server, error) {
	// Phase 1: Walk contentDir to get the set of valid package directories.
	dirs, err := packageDirs(contentDir)
	if err != nil {
		return nil, err
	}

	// Phase 2: For each package directory, parse its files and collect symbols.
	dirFiles := make(map[string][]fileItem)
	fileHTML := make(map[string]template.HTML)
	dirHaveCounts := make(map[string]int)
	dirNeedCounts := make(map[string]int)
	var rawImportPaths []string

	for _, relDir := range dirs {
		importPath := modulePath
		if relDir != "." && relDir != "" {
			importPath = path.Join(modulePath, relDir)
		}
		entries, err := fs.ReadDir(contentDir, relDir)
		if err != nil {
			return nil, err
		}

		var fileNames []string
		fset := token.NewFileSet()
		var astFiles []*ast.File
		fileSrcs := make(map[string][]byte)

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fname := entry.Name()
			// Ignore test files and files that aren't go.
			if !strings.HasSuffix(fname, ".go") || strings.HasSuffix(fname, "_test.go") {
				continue
			}
			fileNames = append(fileNames, fname)

			fp := path.Join(relDir, fname)
			content, err := fs.ReadFile(contentDir, fp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: reading: %v\n", fp, err)
				continue
			}
			fileSrcs[fname] = content

			f, err := parser.ParseFile(fset, fname, content, parser.ParseComments)
			if err == nil {
				astFiles = append(astFiles, f)
			} else {
				fmt.Fprintf(os.Stderr, "%s: parsing: %v\n", fp, err)
			}
		}

		if len(astFiles) == 0 {
			continue
		}

		rawImportPaths = append(rawImportPaths, importPath)

		fileHighlights := make(map[string][]highlight)
		fileNumHave := make(map[string]int)
		fileNumNeed := make(map[string]int)
		pkgNumHave := 0
		pkgNumNeed := 0

		// Call the same function used by the evals page to summarize documentation.
		frontend.CollectSymbols(astFiles, func(id *ast.Ident, has bool) {
			pos := fset.Position(id.Pos())
			end := fset.Position(id.End())

			fileHighlights[pos.Filename] = append(fileHighlights[pos.Filename], highlight{
				start: pos.Offset,
				end:   end.Offset,
				has:   has,
			})
			if has {
				fileNumHave[pos.Filename]++
				pkgNumHave++
			} else {
				fileNumNeed[pos.Filename]++
				pkgNumNeed++
			}
		})

		var fileItems []fileItem
		for _, fname := range fileNames {
			src := fileSrcs[fname]
			key := importPath + "/" + fname
			fileHTML[key] = formatHighlightedFile(src, fileHighlights[fname])

			g := fileNumHave[fname]
			r := fileNumNeed[fname]
			fileItems = append(fileItems, fileItem{
				Filename: fname,
				NumHave:  g,
				NumNeed:  r,
				NeedPct:  percent(g, r),
			})
		}
		slices.SortFunc(fileItems, func(a, b fileItem) int {
			if c := cmp.Compare(b.NumNeed, a.NumNeed); c != 0 {
				return c
			}
			return cmp.Compare(a.Filename, b.Filename)
		})
		dirFiles[importPath] = fileItems
		dirHaveCounts[importPath] = pkgNumHave
		dirNeedCounts[importPath] = pkgNumNeed
	}

	var importPathItems []importPathItem
	for _, ip := range rawImportPaths {
		g := dirHaveCounts[ip]
		r := dirNeedCounts[ip]
		importPathItems = append(importPathItems, importPathItem{
			ImportPath: ip,
			NumHave:    g,
			NumNeed:    r,
			NeedPct:    percent(g, r),
		})
	}

	slices.SortFunc(importPathItems, func(a, b importPathItem) int {
		if c := cmp.Compare(b.NumNeed, a.NumNeed); c != 0 {
			return c
		}
		return cmp.Compare(a.ImportPath, b.ImportPath)
	})

	return &server{
		modulePath:      modulePath,
		resolvedVersion: resolvedVersion,
		importPaths:     importPathItems,
		dirFiles:        dirFiles,
		fileHTML:        fileHTML,
	}, nil
}

// percent returns the percentage of m/ (n + m), rounded to the nearest integer.
// It returns 0 if n + m is 0.
func percent(n, m int) int {
	total := n + m
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(m) * 100 / float64(total)))
}

func formatHighlightedFile(src []byte, highlights []highlight) template.HTML {
	slices.SortFunc(highlights, func(a, b highlight) int {
		return cmp.Compare(a.start, b.start)
	})

	var buf strings.Builder
	last := 0
	for _, h := range highlights {
		if h.start < last || h.start > len(src) || h.end > len(src) {
			continue
		}
		buf.WriteString(template.HTMLEscapeString(string(src[last:h.start])))
		if h.has {
			buf.WriteString(`<span class="has-doc">`)
		} else {
			buf.WriteString(`<span class="need-doc">`)
		}
		buf.WriteString(template.HTMLEscapeString(string(src[h.start:h.end])))
		buf.WriteString(`</span>`)
		last = h.end
	}
	if last < len(src) {
		buf.WriteString(template.HTMLEscapeString(string(src[last:])))
	}

	return template.HTML(buf.String())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.handleRoot(w)
		return
	}
	if r.URL.Path == "/styles.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprint(w, stylesCSS)
		return
	}

	p := strings.TrimPrefix(r.URL.Path, "/")
	if files, ok := s.dirFiles[p]; ok {
		s.handleDir(w, p, files)
		return
	}

	if contentHTML, ok := s.fileHTML[p]; ok {
		s.handleFile(w, p, contentHTML)
		return
	}

	http.NotFound(w, r)
}

const stylesCSS = `
	body { font-family: sans-serif; margin: 20px; }
	.has-doc { color: #1a7f37; font-weight: bold; margin-left: 4px; }
	.need-doc { color: #cf222e; font-weight: bold; margin-left: 4px; }
	.need-pct { color: #000; font-weight: bold; margin-left: 4px; }
	pre {
		font-family: monospace; background-color: #f6f8fa;
	    padding: 16px; border-radius: 6px; overflow: auto;
	}
	.legend { font-family: sans-serif; margin-bottom: 1em; }
	pre .has-doc, .legend .has-doc {
		background-color: #dafbe1; padding: 0 2px; border-radius: 2px; margin-left: 0;
	}
	pre .need-doc, .legend .need-doc {
		background-color: #ffebe9; padding: 0 2px; border-radius: 2px; margin-left: 0;
	}
`

var rootTmpl = template.Must(template.New("root").Parse(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>Module {{.ModulePath}}@{{.Version}}</title>
	<link rel="stylesheet" href="/styles.css">
</head>
<body>
	<h1>Module {{.ModulePath}}@{{.Version}}
	    <span class="has-doc">{{.NumHave}}</span>
	    <span class="need-doc">{{.NumNeed}}</span>
	    <span class="need-pct">{{.NeedPct}}%</span>
	</h1>
	<h2>Packages</h2>
	<ul>
		{{range .ImportPaths}}
			<li>
				<a href="/{{.ImportPath}}">{{.ImportPath}}</a>
				<span class="has-doc">{{.NumHave}}</span>
				<span class="need-doc">{{.NumNeed}}</span>
				<span class="need-pct">{{.NeedPct}}%</span>
			</li>
		{{end}}
	</ul>
</body>
</html>`))

func (s *server) handleRoot(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var have, need int
	for _, item := range s.importPaths {
		have += item.NumHave
		need += item.NumNeed
	}
	data := struct {
		ModulePath       string
		Version          string
		NumHave, NumNeed int
		NeedPct          int
		ImportPaths      []importPathItem
	}{
		ModulePath:  s.modulePath,
		Version:     s.resolvedVersion,
		NumHave:     have,
		NumNeed:     need,
		NeedPct:     percent(have, need),
		ImportPaths: s.importPaths,
	}
	rootTmpl.Execute(w, data)
}

var dirTmpl = template.Must(template.New("dir").Parse(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>Files in {{.ImportPath}}</title>
	<link rel="stylesheet" href="/styles.css">
</head>
<body>
	<p><a href="/">
	     Back to Packages</a></p>
	<h1>Files in {{.ImportPath}}</h1>
	<ul>
		{{range .Files}}
			<li>
				<a href="/{{$.ImportPath}}/{{.Filename}}">{{.Filename}}</a>
				<span class="has-doc">{{.NumHave}}</span>
				<span class="need-doc">{{.NumNeed}}</span>
				<span class="need-pct">{{.NeedPct}}%</span>
			</li>
		{{end}}
	</ul>
</body>
</html>`))

func (s *server) handleDir(w http.ResponseWriter, importPath string, files []fileItem) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		ImportPath string
		Files      []fileItem
	}{
		ImportPath: importPath,
		Files:      files,
	}
	dirTmpl.Execute(w, data)
}

var fileTmpl = template.Must(template.New("file").Parse(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>{{.Filename}} - {{.ImportPath}}</title>
	<link rel="stylesheet" href="/styles.css">
</head>
<body>
	<p><a href="/{{.ImportPath}}">← Back to {{.ImportPath}}</a></p>
	<div class="legend">
		Symbol status:
		<span class="has-doc">Documented (Green)</span> |
		<span class="need-doc">Undocumented (Red)</span>
	</div>
	<h1>{{.Filename}}</h1>
	<pre><code>{{.Contents}}</code></pre>
</body>
</html>`))

func (s *server) handleFile(w http.ResponseWriter, key string, contentHTML template.HTML) {
	var importPath, filename string
	for ip := range s.dirFiles {
		if after, ok := strings.CutPrefix(key, ip+"/"); ok {
			if len(ip) > len(importPath) {
				importPath = ip
				filename = after
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fileData := struct {
		ImportPath string
		Filename   string
		Contents   template.HTML
	}{
		ImportPath: importPath,
		Filename:   filename,
		Contents:   contentHTML,
	}
	fileTmpl.Execute(w, fileData)
}
