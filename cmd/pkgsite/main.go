// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Pkgsite extracts and generates documentation for Go programs.
// It runs as a web server and presents the documentation as a
// web page.
//
// To install, run `go install golang.org/x/pkgsite/cmd/pkgsite@latest`.
//
// With no arguments, pkgsite will serve docs for main modules relative to the
// current directory, i.e. the modules listed by `go list -m`. This is
// typically the module defined by the nearest go.mod file in a parent
// directory. However, this may include multiple main modules when using a
// go.work file to define a [workspace].
//
// For example, both of the following forms could be used to work
// on the module defined in repos/cue/go.mod:
//
// The single module form:
//
//	cd repos/cue && pkgsite
//
// The multiple module form:
//
//	go work init repos/cue repos/other && pkgsite
//
// By default, the resulting server will also serve all of the module's
// dependencies at their required versions. You can disable serving the
// required modules by passing -list=false.
//
// You can also serve docs from your module cache, directly from the proxy
// (it uses the GOPROXY environment variable), or both:
//
//	pkgsite -cache -proxy
//
// With either -cache or -proxy, pkgsite won't look for a module in the current
// directory. You can still provide modules on the local filesystem by listing
// their paths:
//
//	pkgsite -cache -proxy ~/repos/cue some/other/module
//
// Although standard library packages will work by default, the docs can take a
// while to appear the first time because the Go repo must be cloned and
// processed. If you clone the repo yourself (https://go.googlesource.com/go),
// you can provide its location with the -gorepo flag to save a little time.
//
// [workspace]: https://go.dev/ref/mod#workspaces
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/browser"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/static"
	thirdparty "golang.org/x/pkgsite/third_party"
)

const defaultAddr = "localhost:8080" // default webserver address

var (
	httpAddr   = flag.String("http", defaultAddr, "HTTP service address to listen for incoming requests on")
	goRepoPath = flag.String("gorepo", "", "path to Go repo on local filesystem")
	useProxy   = flag.Bool("proxy", false, "fetch from GOPROXY if not found locally")
	devMode    = flag.Bool("dev", false, "enable developer mode (reload templates on each page load, serve non-minified JS/CSS, etc.)")
	staticFlag = flag.String("static", "static", "path to folder containing static files served")
	openFlag   = flag.Bool("open", false, "open a browser window to the server's address")
	// other flags are bound to serverConfig below
)

type serverConfig struct {
	paths         []string
	gopathMode    bool
	useCache      bool
	cacheDir      string
	useListedMods bool

	proxy *proxy.Client // client, or nil; controlled by the -proxy flag
}

func main() {
	var serverCfg serverConfig

	flag.BoolVar(&serverCfg.gopathMode, "gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src")
	flag.BoolVar(&serverCfg.useCache, "cache", false, "fetch from the module cache")
	flag.StringVar(&serverCfg.cacheDir, "cachedir", "", "module cache directory (defaults to `go env GOMODCACHE`)")
	flag.BoolVar(&serverCfg.useListedMods, "list", true, "for each path, serve all modules in build list")

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: %s [flags] [PATHS ...]\n", os.Args[0])
		fmt.Fprintf(out, "    where each PATHS is a single path or a comma-separated list\n")
		fmt.Fprintf(out, "    (default is current directory if neither -cache nor -proxy is provided)\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	serverCfg.paths = collectPaths(flag.Args())

	if serverCfg.useCache || *useProxy {
		fmt.Fprintf(os.Stderr, "BYPASSING LICENSE CHECKING: MAY DISPLAY NON-REDISTRIBUTABLE INFORMATION\n")
	}

	if *useProxy {
		url := os.Getenv("GOPROXY")
		if url == "" {
			die("GOPROXY environment variable is not set")
		}
		var err error
		serverCfg.proxy, err = proxy.New(url)
		if err != nil {
			die("connecting to proxy: %s", err)
		}
	}

	if *goRepoPath != "" {
		stdlib.SetGoRepoPath(*goRepoPath)
	}

	ctx := context.Background()
	server, err := buildServer(ctx, serverCfg)
	if err != nil {
		die(err.Error())
	}

	addr := *httpAddr
	if addr == "" {
		addr = ":http"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		die(err.Error())
	}

	url := "http://" + addr
	log.Infof(ctx, "Listening on addr %s", url)

	if *openFlag {
		go func() {
			if !browser.Open(url) {
				log.Infof(ctx, "Failed to open browser window. Please visit %s in your browser.", url)
			}
		}()
	}

	router := http.NewServeMux()
	server.Install(router.Handle, nil, nil)
	mw := middleware.Timeout(54 * time.Second)
	srv := &http.Server{Addr: addr, Handler: mw(router)}
	die("%v", srv.Serve(ln))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	os.Exit(1)
}

func buildServer(ctx context.Context, serverCfg serverConfig) (*frontend.Server, error) {
	if len(serverCfg.paths) == 0 && !serverCfg.useCache && serverCfg.proxy == nil {
		serverCfg.paths = []string{"."}
	}

	cfg := getterConfig{
		all:   serverCfg.useListedMods,
		proxy: serverCfg.proxy,
	}

	// By default, the requested paths are interpreted as directories. However,
	// if -gopath_mode is set, they are interpreted as relative paths to modules
	// in a GOPATH directory.
	if serverCfg.gopathMode {
		var err error
		cfg.dirs, err = getGOPATHModuleDirs(ctx, serverCfg.paths)
		if err != nil {
			return nil, fmt.Errorf("searching GOPATH: %v", err)
		}
	} else {
		var err error
		cfg.dirs, err = getModuleDirs(ctx, serverCfg.paths)
		if err != nil {
			return nil, fmt.Errorf("searching GOPATH: %v", err)
		}
	}

	if serverCfg.useCache {
		cfg.modCacheDir = serverCfg.cacheDir
		if cfg.modCacheDir == "" {
			var err error
			cfg.modCacheDir, err = defaultCacheDir()
			if err != nil {
				return nil, err
			}
			if cfg.modCacheDir == "" {
				return nil, fmt.Errorf("empty value for GOMODCACHE")
			}
		}
	}

	getters, err := buildGetters(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Collect unique module paths served by this server.
	seenModules := make(map[frontend.LocalModule]bool)
	var allModules []frontend.LocalModule
	for _, modules := range cfg.dirs {
		for _, m := range modules {
			if seenModules[m] {
				continue
			}
			seenModules[m] = true
			allModules = append(allModules, m)
		}
	}
	sort.Slice(allModules, func(i, j int) bool {
		return allModules[i].ModulePath < allModules[j].ModulePath
	})

	return newServer(getters, allModules, cfg.proxy)
}

func collectPaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		paths = append(paths, strings.Split(arg, ",")...)
	}
	return paths
}

// getGOPATHModuleDirs returns the set of workspace modules for each directory,
// determined by running go list -m.
//
// An error is returned if any operations failed unexpectedly, or if no
// requested directories contain any valid modules.
func getModuleDirs(ctx context.Context, dirs []string) (map[string][]frontend.LocalModule, error) {
	dirModules := make(map[string][]frontend.LocalModule)
	for _, dir := range dirs {
		output, err := runGo(dir, "list", "-m", "-json")
		if err != nil {
			return nil, fmt.Errorf("listing modules in %s: %v", dir, err)
		}
		var modules []frontend.LocalModule
		decoder := json.NewDecoder(bytes.NewBuffer(output))
		for decoder.More() {
			var m frontend.LocalModule
			if err := decoder.Decode(&m); err != nil {
				return nil, err
			}
			if m.ModulePath != "command-line-arguments" {
				modules = append(modules, m)
			}
		}
		if len(modules) > 0 {
			dirModules[dir] = modules
		}
	}
	if len(dirs) > 0 && len(dirModules) == 0 {
		return nil, fmt.Errorf("no modules in any of the requested directories")
	}
	return dirModules, nil
}

// getGOPATHModuleDirs returns local module information for directories in
// GOPATH corresponding to the requested module paths.
//
// An error is returned if any operations failed unexpectedly, or if no modules
// were resolved. If individual module paths are not found, an error is logged
// and the path skipped.
func getGOPATHModuleDirs(ctx context.Context, modulePaths []string) (map[string][]frontend.LocalModule, error) {
	gopath, err := runGo("", "env", "GOPATH")
	if err != nil {
		return nil, err
	}
	gopaths := filepath.SplitList(strings.TrimSpace(string(gopath)))

	dirs := make(map[string][]frontend.LocalModule)
	for _, path := range modulePaths {
		dir := ""
		for _, gopath := range gopaths {
			candidate := filepath.Join(gopath, "src", path)
			info, err := os.Stat(candidate)
			if err == nil && info.IsDir() {
				dir = candidate
				break
			}
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
		if dir == "" {
			log.Errorf(ctx, "ERROR: no GOPATH directory contains %q", path)
		} else {
			dirs[dir] = []frontend.LocalModule{{ModulePath: path, Dir: dir}}
		}
	}

	if len(modulePaths) > 0 && len(dirs) == 0 {
		return nil, fmt.Errorf("no GOPATH directories contain any of the requested module(s)")
	}
	return dirs, nil
}

// getterConfig defines the set of getters for the server to use.
// See buildGetters.
type getterConfig struct {
	all         bool                              // if set, request "all" instead of ["<modulePath>/..."]
	dirs        map[string][]frontend.LocalModule // local modules to serve
	modCacheDir string                            // path to module cache, or ""
	proxy       *proxy.Client                     // proxy client, or nil
}

// buildGetters constructs module getters based on the given configuration.
//
// Getters are returned in the following priority order:
//  1. local getters for cfg.dirs, in the given order
//  2. a module cache getter, if cfg.modCacheDir != ""
//  3. a proxy getter, if cfg.proxy != nil
func buildGetters(ctx context.Context, cfg getterConfig) ([]fetch.ModuleGetter, error) {
	var getters []fetch.ModuleGetter

	// Load local getters for each directory.
	for dir, modules := range cfg.dirs {
		var patterns []string
		if cfg.all {
			patterns = append(patterns, "all")
		} else {
			for _, m := range modules {
				patterns = append(patterns, fmt.Sprintf("%s/...", m))
			}
		}
		mg, err := fetch.NewGoPackagesModuleGetter(ctx, dir, patterns...)
		if err != nil {
			log.Errorf(ctx, "Loading packages from %s: %v", dir, err)
		} else {
			getters = append(getters, mg)
		}
	}
	if len(getters) == 0 && len(cfg.dirs) > 0 {
		return nil, fmt.Errorf("failed to load any module(s) at %v", cfg.dirs)
	}

	// Add a getter for the local module cache.
	if cfg.modCacheDir != "" {
		g, err := fetch.NewModCacheGetter(cfg.modCacheDir)
		if err != nil {
			return nil, err
		}
		getters = append(getters, g)
	}

	// Add a proxy
	if cfg.proxy != nil {
		getters = append(getters, fetch.NewProxyModuleGetter(cfg.proxy, source.NewClient(time.Second)))
	}
	return getters, nil
}

func newServer(getters []fetch.ModuleGetter, localModules []frontend.LocalModule, prox *proxy.Client) (*frontend.Server, error) {
	lds := fetchdatasource.Options{
		Getters:              getters,
		ProxyClientForLatest: prox,
		BypassLicenseCheck:   true,
	}.New()

	// In dev mode, use a dirFS to pick up template/JS/CSS changes without
	// restarting the server.
	var staticFS fs.FS
	if *devMode {
		staticFS = os.DirFS(*staticFlag)
	} else {
		staticFS = static.FS
	}

	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: func(context.Context) internal.DataSource { return lds },
		TemplateFS:       template.TrustedFSFromEmbed(static.FS),
		StaticFS:         staticFS,
		DevMode:          *devMode,
		LocalMode:        true,
		LocalModules:     localModules,
		StaticPath:       *staticFlag,
		ThirdPartyFS:     thirdparty.FS,
	})
	if err != nil {
		return nil, err
	}
	for _, g := range getters {
		p, fsys := g.SourceFS()
		if p != "" {
			server.InstallFS(p, fsys)
		}
	}
	return server, nil
}

func defaultCacheDir() (string, error) {
	out, err := runGo("", "env", "GOMODCACHE")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runGo(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running go with %q: %v: %s", args, err, out)
	}
	return out, nil
}
