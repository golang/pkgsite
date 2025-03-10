// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgsite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/static"
	thirdparty "golang.org/x/pkgsite/third_party"
)

// ServerConfig provides configuration for BuildServer.
type ServerConfig struct {
	Paths            []string
	GOPATHMode       bool
	UseCache         bool
	CacheDir         string
	UseListedMods    bool
	UseLocalStdlib   bool
	DevMode          bool
	DevModeStaticDir string
	GoRepoPath       string

	Proxy *proxy.Client // client, or nil; controlled by the -proxy flag
}

// BuildServer builds a *frontend.Server using the given configuration.
func BuildServer(ctx context.Context, serverCfg ServerConfig) (*frontend.Server, error) {
	if len(serverCfg.Paths) == 0 && !serverCfg.UseCache && serverCfg.Proxy == nil {
		serverCfg.Paths = []string{"."}
	}

	cfg := getterConfig{
		all:        serverCfg.UseListedMods,
		proxy:      serverCfg.Proxy,
		goRepoPath: serverCfg.GoRepoPath,
	}

	// By default, the requested Paths are interpreted as directories. However,
	// if -gopath_mode is set, they are interpreted as relative Paths to modules
	// in a GOPATH directory.
	if serverCfg.GOPATHMode {
		var err error
		cfg.dirs, err = getGOPATHModuleDirs(ctx, serverCfg.Paths)
		if err != nil {
			return nil, fmt.Errorf("searching GOPATH: %v", err)
		}
	} else {
		var err error
		cfg.dirs, err = getModuleDirs(ctx, serverCfg.Paths)
		if err != nil {
			return nil, fmt.Errorf("searching modules: %v", err)
		}
	}

	if serverCfg.UseCache {
		cfg.modCacheDir = serverCfg.CacheDir
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

	if serverCfg.UseLocalStdlib {
		cfg.useLocalStdlib = true
	}

	getters, err := buildGetters(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Collect unique module Paths served by this server.
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

	return newServer(getters, allModules, cfg.proxy, serverCfg.DevMode, serverCfg.DevModeStaticDir)
}

// getModuleDirs returns the set of workspace modules for each directory,
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
// GOPATH corresponding to the requested module Paths.
//
// An error is returned if any operations failed unexpectedly, or if no modules
// were resolved. If individual module Paths are not found, an error is logged
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
	all            bool                              // if set, request "all" instead of ["<modulePath>/..."]
	dirs           map[string][]frontend.LocalModule // local modules to serve
	modCacheDir    string                            // path to module cache, or ""
	proxy          *proxy.Client                     // proxy client, or nil
	useLocalStdlib bool                              // use go/packages for the local stdlib
	goRepoPath     string                            // repo path for local stdlib
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

	if cfg.useLocalStdlib {
		goRepo := cfg.goRepoPath
		if goRepo == "" {
			goRepo = internal.GOROOT()
		}
		if goRepo != "" { // if goRepo == "" we didn't get a *goRepoPath and couldn't find GOROOT. Fall back to the zip files.
			mg, err := fetch.NewGoPackagesStdlibModuleGetter(ctx, goRepo)
			if err != nil {
				log.Errorf(ctx, "loading packages from stdlib: %v", err)
			} else {
				getters = append(getters, mg)
			}
		}
	}

	// Add a proxy
	if cfg.proxy != nil {
		getters = append(getters, fetch.NewProxyModuleGetter(cfg.proxy, source.NewClient(&http.Client{Timeout: time.Second})))
	}

	getters = append(getters, fetch.NewStdlibZipModuleGetter())

	return getters, nil
}

func newServer(getters []fetch.ModuleGetter, localModules []frontend.LocalModule, prox *proxy.Client, devMode bool, staticFlag string) (*frontend.Server, error) {
	lds := fetchdatasource.Options{
		Getters:              getters,
		ProxyClientForLatest: prox,
		BypassLicenseCheck:   true,
	}.New()

	// In dev mode, use a dirFS to pick up template/JS/CSS changes without
	// restarting the server.
	var staticFS fs.FS
	if devMode {
		staticFS = os.DirFS(staticFlag)
	} else {
		staticFS = static.FS
	}

	// Preload local modules to warm the cache.
	for _, lm := range localModules {
		go lds.GetUnitMeta(context.Background(), "", lm.ModulePath, fetch.LocalVersion)
	}
	go lds.GetUnitMeta(context.Background(), "", "std", "latest")

	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: func(context.Context) internal.DataSource { return lds },
		TemplateFS:       template.TrustedFSFromEmbed(static.FS),
		StaticFS:         staticFS,
		DevMode:          devMode,
		LocalMode:        true,
		LocalModules:     localModules,
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
		if ee, ok := err.(*exec.ExitError); ok {
			out = append(out, ee.Stderr...)
		}
		return nil, fmt.Errorf("running go with %q: %v: %s", args, err, out)
	}
	return out, nil
}
