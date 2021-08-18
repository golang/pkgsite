// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxytest supports testing with the proxy.
package proxytest

import "fmt"

// Module represents a module version used by the proxy server.
type Module struct {
	ModulePath string
	Version    string
	Files      map[string]string
	NotCached  bool // if true, behaves like it's uncached
	zip        []byte
}

// ChangePath returns a copy of m with a different module path.
func (m *Module) ChangePath(modulePath string) *Module {
	m2 := *m
	m2.ModulePath = modulePath
	return &m2
}

// ChangeVersion returns a copy of m with a different version.
func (m *Module) ChangeVersion(version string) *Module {
	m2 := *m
	m2.Version = version
	return &m2
}

// AddFile returns a copy of m with an additional file. It
// panics if the filename is already present.
func (m *Module) AddFile(filename, contents string) *Module {
	return m.setFile(filename, &contents, false)
}

// DeleteFile returns a copy of m with filename removed.
// It panics if filename is not present.
func (m *Module) DeleteFile(filename string) *Module {
	return m.setFile(filename, nil, true)
}

// ReplaceFile returns a copy of m with different contents for filename.
// It panics if filename is not present.
func (m *Module) ReplaceFile(filename, contents string) *Module {
	return m.setFile(filename, &contents, true)
}

func (m *Module) setFile(filename string, contents *string, mustExist bool) *Module {
	_, ok := m.Files[filename]
	if mustExist && !ok {
		panic(fmt.Sprintf("%s@%s does not have a file named %s", m.ModulePath, m.Version, filename))
	}
	if !mustExist && ok {
		panic(fmt.Sprintf("%s@%s already has a file named %s", m.ModulePath, m.Version, filename))
	}
	m2 := *m
	if m.Files != nil {
		m2.Files = map[string]string{}
		for k, v := range m.Files {
			m2.Files[k] = v
		}
	}
	if contents == nil {
		delete(m2.Files, filename)
	} else {
		m2.Files[filename] = *contents
	}
	return &m2
}
