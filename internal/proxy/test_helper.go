// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/tools/txtar"
)

// SetupTestClient creates a fake module proxy for testing using the given test
// version information.
//
// It returns a function for tearing down the proxy after the test is completed
// and a Client for interacting with the test proxy.
func SetupTestClient(t *testing.T, modules []*Module) (*Client, func()) {
	t.Helper()
	s := NewServer(modules)
	client, serverClose, err := NewClientForServer(s)
	if err != nil {
		t.Fatal(err)
	}
	return client, serverClose
}

// NewClientForServer starts serving proxyMux locally. It returns a client to the
// server and a function to shut down the server.
func NewClientForServer(s *Server) (*Client, func(), error) {
	// override client.httpClient to skip TLS verification
	httpClient, proxy, serverClose := testhelper.SetupTestClientAndServer(s.mux)
	client, err := New(proxy.URL)
	if err != nil {
		return nil, nil, err
	}
	client.httpClient = httpClient
	return client, serverClose, nil
}

// LoadTestModules reads the modules in the given directory. Each file in that
// directory with a .txtar extension should be named "path@version" and should
// be in txtar format (golang.org/x/tools/txtar). The path part of the filename
// will be preceded by "example.com/" and colons will be replaced by slashes to
// form a full module path. The file contents are used verbatim except that some
// variables beginning with "$" are substituted with predefined strings.
//
// LoadTestModules panics if there is an error reading any of the files.
func LoadTestModules(dir string) []*Module {
	files, err := filepath.Glob(filepath.Join(dir, "*.txtar"))
	if err != nil {
		panic(err)
	}
	var ms []*Module
	for _, f := range files {
		m, err := readTxtarModule(f)
		if err != nil {
			panic(err)
		}
		ms = append(ms, m)
	}
	return ms
}

var testModuleReplacer = strings.NewReplacer(
	"$MITLicense", testhelper.MITLicense,
	"$BSD0License", testhelper.BSD0License,
)

func readTxtarModule(filename string) (*Module, error) {
	modver := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	i := strings.IndexRune(modver, '@')
	if i < 0 {
		return nil, fmt.Errorf("%s: filename missing '@'", modver)
	}
	modulePath, version := "example.com/"+modver[:i], modver[i+1:]
	modulePath = strings.ReplaceAll(modulePath, ":", "/")
	if modulePath == "" || version == "" {
		return nil, fmt.Errorf("%s: empty module path or version", filename)
	}
	m := &Module{
		ModulePath: modulePath,
		Version:    version,
		Files:      map[string]string{},
	}
	ar, err := txtar.ParseFile(filename)
	if err != nil {
		return nil, err
	}
	for _, f := range ar.Files {
		m.Files[f.Name] = strings.TrimSpace(testModuleReplacer.Replace(string(f.Data)))
	}
	return m, nil
}

// FindModule returns the module in mods with the given path and version, or nil
// if there isn't one. An empty version argument matches any version.
func FindModule(mods []*Module, path, version string) *Module {
	for _, m := range mods {
		if m.ModulePath == path && (version == "" || m.Version == version) {
			return m
		}
	}
	return nil
}
