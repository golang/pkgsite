// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// stdlibsymbol compares database information with
// the stdlib API data at
// https://go.googlesource.com/go/+/refs/heads/master/api.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/symbol"
)

var (
	path = flag.String("path", "", "path to compare against dataset")
)

func main() {
	flag.Parse()

	log.SetLevel("error")
	ctx := context.Background()
	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	if *path != "" {
		if err := compareVersionsPage(*path); err != nil {
			log.Fatal(ctx, err)
		}
		return
	}
	if err := compareDB(ctx, cfg); err != nil {
		log.Fatal(ctx, err)
	}
}

func compareDB(ctx context.Context, cfg *config.Config) error {
	db, err := cmdconfig.OpenDB(ctx, cfg, false)
	if err != nil {
		return err
	}
	pkgToErrors, err := db.CompareStdLib(ctx)
	if err != nil {
		return err
	}
	for path, errs := range pkgToErrors {
		fmt.Printf("----- %s -----\n", path)
		for _, e := range errs {
			fmt.Print(e)
		}
	}
	return nil
}

func compareVersionsPage(path string) error {
	if os.Getenv("GO_DISCOVERY_SERVE_STATS") != "true" {
		fmt.Println("Ensure that GO_DISCOVERY_SERVE_STATS=true in the window running the frontend, otherwise the JSON endpoint will be disabled.")
	}

	url := fmt.Sprintf("http://localhost:8080/%s?tab=versions&m=json", path)
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var vd frontend.VersionsDetails
	if err := json.Unmarshal(body, &vd); err != nil {
		return fmt.Errorf("json.Unmarshal: %v", err)
	}

	versionToNameToSymbol := map[string]map[string]*internal.UnitSymbol{}
	for _, vl := range vd.ThisModule {
		for _, vs := range vl.Versions {
			v := stdlib.VersionForTag(vs.Version)
			versionToNameToSymbol[v] = map[string]*internal.UnitSymbol{}
			for _, s := range vs.Symbols {
				if s.New {
					versionToNameToSymbol[v][s.Name] = unitSymbol(s)
				}
				for _, c := range s.Children {
					versionToNameToSymbol[v][c.Name] = unitSymbol(c)
				}
			}
		}
	}
	apiVersions, err := symbol.ParsePackageAPIInfo()
	if err != nil {
		return err
	}
	errs := symbol.CompareStdLib(path, apiVersions[path], versionToNameToSymbol)
	for _, e := range errs {
		fmt.Print(e)
	}
	return nil
}

func unitSymbol(s *frontend.Symbol) *internal.UnitSymbol {
	us := &internal.UnitSymbol{Name: s.Name}
	if len(s.Builds) == 0 {
		us.AddBuildContext(internal.BuildContextAll)
	}
	for _, b := range s.Builds {
		parts := strings.SplitN(b, "/", 2)
		var build internal.BuildContext
		switch parts[0] {
		case "linux":
			build = internal.BuildContextLinux
		case "darwin":
			build = internal.BuildContextDarwin
		case "windows":
			build = internal.BuildContextWindows
		case "js":
			build = internal.BuildContextJS
		}
		if us.SupportsBuild(build) {
			fmt.Printf("duplicate build context for %q: %v\n", s.Name, build)
		}
		us.AddBuildContext(build)
	}
	return us
}
