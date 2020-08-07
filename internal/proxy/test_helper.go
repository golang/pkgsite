// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"testing"

	"golang.org/x/pkgsite/internal/testing/testhelper"
)

// SetupTestClient creates a fake module proxy for testing using the given test
// version information. If modules is nil, it will default to hosting the
// modules in the testdata directory.
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
