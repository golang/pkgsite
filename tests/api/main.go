// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command api computes the exported API of a set of Go packages.
package main

import (
	"archive/zip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/symbol"
	"golang.org/x/pkgsite/internal/version"
)

var (
	compareAll = flag.Bool("all", false,
		"compare all packages in tests/api/testdata if true")
	frontendHost = flag.String("frontend", "http://localhost:8080",
		"Use the frontend host referred to by this URL for comparing data")
	proxyURL = flag.String("proxy", "https://proxy.golang.org",
		"Use the module proxy referred to by this URL for fetching packages")
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "api [cmd] [module path]:[package path suffix]")
		fmt.Fprintf(out, "  generate: generates the API history for a package and writes to %s\n", testdataDir)
		fmt.Fprintf(out, "  compare: compares the API history for a package in %s to %s\n", testdataDir, *frontendHost)
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *compareAll {
		if flag.NArg() != 1 {
			flag.Usage()
			log.Fatalf("unexpected number of arguments for -all: %v", flag.Args())
		}
	} else if flag.NArg() != 2 {
		flag.Usage()
		log.Fatalf("unexpected number of arguments: %v", flag.Args())
	}

	ctx := context.Background()
	cmd := flag.Args()[0]
	var pkgPath, modulePath string
	if !*compareAll {
		pkgPath, modulePath = parsePath(flag.Args()[1])
	}
	if err := run(ctx, cmd, pkgPath, modulePath, *frontendHost, *proxyURL, *compareAll); err != nil {
		log.Fatal(err)
	}
}

func parsePath(arg string) (pkgPath, modulePath string) {
	if !strings.Contains(arg, ":") {
		return arg, arg
	}
	parts := strings.SplitN(arg, ":", 2)
	return strings.Join(parts, "/"), parts[0]
}

const (
	testdataDir = "tests/api/testdata"
	tmpDir      = "/tmp/api"
)

func run(ctx context.Context, cmd, pkgPath, modulePath, frontendHost, proxyURL string, compareAll bool) error {
	switch cmd {
	case "compare":
		if compareAll {
			pkgPaths, err := allPackages()
			if err != nil {
				return err
			}
			for _, p := range pkgPaths {
				if err := compare(frontendHost, p); err != nil {
					return err
				}
			}
			return nil
		}
		return compare(frontendHost, pkgPath)
	case "generate":
		return generate(ctx, pkgPath, modulePath, tmpDir, proxyURL)
	}
	return fmt.Errorf("unsupported command: %q", cmd)
}

func generate(ctx context.Context, pkgPath, modulePath, tmpPath, proxyURL string) (err error) {
	defer derrors.Wrap(&err, "generate(ctx, %q, %q, %q, %q)", pkgPath, modulePath, tmpPath, proxyURL)
	proxyClient, err := proxy.New(proxyURL)
	if err != nil {
		return err
	}
	versions, err := proxyClient.Versions(ctx, modulePath)
	if err != nil {
		return err
	}
	versions = sortVersion(versions)
	fmt.Printf("Processing %d versions\n\n", len(versions))
	prevFeatureSet := map[string]bool{}
	for _, ver := range versions {
		typ, err := version.ParseType(ver)
		if err != nil {
			return err
		}
		if typ != version.TypeRelease {
			continue
		}
		if version.IsIncompatible(ver) {
			continue
		}
		featureCtx, err := fetchFeatureContext(ctx, proxyClient, modulePath, pkgPath, ver, tmpPath)
		if errors.Is(err, derrors.NotFound) {
			continue
		}
		if err != nil {
			return err
		}
		newFeatures, featureSet := symbol.FeaturesForVersion(featureCtx, prevFeatureSet)
		prevFeatureSet = featureSet
		if len(newFeatures) == 0 {
			fmt.Println("No features for this version.")
			continue
		}
		if err := writeFeatures(newFeatures, pkgPath, ver, testdataDir); err != nil {
			return err
		}
	}
	return nil
}

// allPackages returns all package paths in tests/api/testdata.
func allPackages() (_ []string, err error) {
	defer derrors.Wrap(&err, "allPackages")
	dirToFiles := map[string][]string{}
	err = filepath.Walk(
		"tests/api/testdata",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			dir := filepath.Dir(path)
			dirToFiles[dir] = append(dirToFiles[dir], path)
			return nil
		})
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range dirToFiles {
		paths = append(paths, strings.TrimPrefix(p, testdataDir+"/"))
	}
	return paths, nil
}

// compare compares data from the testdata directory with the frontend.
func compare(frontendHost, pkgPath string) (err error) {
	defer derrors.Wrap(&err, "compare(ctx, %q, %q, %q)", frontendHost, pkgPath, testdataDir)
	files, err := symbol.LoadAPIFiles(pkgPath, testdataDir)
	if err != nil {
		return err
	}
	apiVersions, err := symbol.ParsePackageAPIInfo(files)
	if err != nil {
		return err
	}

	// Parse API data from the frontend versions page.
	client := frontend.NewClient(frontendHost)
	vd, err := client.GetVersions(pkgPath)
	if err != nil {
		return err
	}

	sh, err := frontend.ParseVersionsDetails(vd)
	if err != nil {
		return err
	}

	// Compare the output of these two data sources.
	errors, err := symbol.CompareAPIVersions(pkgPath, apiVersions[pkgPath], sh)
	if err != nil {
		return err
	}
	if len(errors) == 0 {
		fmt.Printf("The APIs match for %s!\n", pkgPath)
		return nil
	}
	fmt.Printf("---------- Errors for %s\n", pkgPath)
	for _, e := range errors {
		fmt.Print(e)
	}
	return nil
}

func fetchFeatureContext(ctx context.Context, proxyClient *proxy.Client,
	modulePath, pkgPath, ver, dirPath string) (_ map[string]map[string]bool, err error) {
	defer derrors.Wrap(&err, "fetchFeatureContext(ctx, proxyClient, %q, %q, %q, %q)",
		modulePath, pkgPath, ver, dirPath)
	r, err := proxyClient.Zip(ctx, modulePath, ver)
	if err != nil {
		return nil, err
	}
	if err := writeZip(r, dirPath); err != nil {
		return nil, err
	}
	modver := fmt.Sprintf("%s@%s", modulePath, ver)
	modDir := fmt.Sprintf("%s/%s", dirPath, modver)
	pkgDir := fmt.Sprintf("%s/%s", modDir, internal.Suffix(pkgPath, modulePath))
	fmt.Printf("----- %s ----- (source: %s)\n", ver, pkgDir)

	if fi, err := os.Stat(pkgDir); err != nil || !fi.IsDir() {
		fmt.Printf("package %q is not present in this version\n", pkgPath)
		return nil, derrors.NotFound
	}

	modFile := modDir + "/go.mod"
	if _, err := os.Stat(modFile); os.IsNotExist(err) {
		fmt.Printf("%q not found; running go mod init in %q\n", modFile, modDir)
		cmd := exec.Command("go", "mod", "init", modulePath)
		cmd.Dir = modDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("error running go mod init: %v \n %s", err, string(out))
		}
	}
	return symbol.GenerateFeatureContexts(ctx, pkgPath, pkgDir)
}

func writeFeatures(features []string, pkgPath, ver, outDir string) (err error) {
	defer derrors.Wrap(&err, "writeFeatures(%v, %q, %q, %q)", features, pkgPath, ver, outDir)
	if outDir == "" {
		sort.Strings(features)
		for _, s := range features {
			fmt.Println(s)
		}
		return nil
	}
	out := fmt.Sprintf("%s/%s/%s.txt", outDir, pkgPath, ver)
	if err := os.MkdirAll(filepath.Dir(out), os.ModePerm); err != nil {
		return fmt.Errorf("os.MkdirAll: %v", err)
	}
	file, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("os.Create: %v", err)
	}
	defer func() {
		cerr := file.Close()
		if err == nil {
			err = cerr
		}
	}()

	sort.Strings(features)
	for _, s := range features {
		if _, err := file.WriteString(s + "\n"); err != nil {
			return err
		}
	}
	fmt.Printf("Found %d features\n", len(features))
	return nil
}

func writeZip(r *zip.Reader, destination string) (err error) {
	defer derrors.Wrap(&err, "writeZip(r, %q)", destination)
	for _, f := range r.File {
		fpath := filepath.Join(destination, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("%s is an illegal filepath", fpath)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			return err
		}
		if err := outFile.Close(); err != nil {
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
	}
	return nil
}

func sortVersion(versions []string) []string {
	sort.Slice(versions, func(i, j int) bool {
		return semver.Compare(versions[i], versions[j]) < 0
	})
	return versions
}
