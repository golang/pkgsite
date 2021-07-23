/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/*
TODO: replace these cases with something in seeddb
  ${'package with multiple nested modules'}                        | ${'/github.com/Azure/go-autorest/autorest@v0.11.18#section-directories'}
  ${'page that will redirect'}                                     | ${'/github.com/jackc/pgx/pgxpool'}
*/

const isCI = process.env.CI;

// Temporarily skipping unit page tests until versions page
// tests are separated in a followup CL.
export const testcases = (isCI ? test.skip.each : test.each)`
  name                                                             | path
  ${'standard library'}                                            | ${'/std@go1.16.3'}
  ${'standard library versions'}                                   | ${'/std?tab=versions'}
  ${'standard library licenses'}                                   | ${'/std@go1.16.3?tab=licenses'}
  ${'errors versions'}                                             | ${'/errors?tab=versions'}
  ${'errors licenses'}                                             | ${'/errors@go1.16.3?tab=licenses'}
  ${'errors imports'}                                              | ${'/errors@go1.16.3?tab=imports'}
  ${'tools'}                                                       | ${'/golang.org/x/tools@v0.1.1'}
  ${'tools licenses'}                                              | ${'/golang.org/x/tools@v0.1.1?tab=licenses'}
  ${'module that is not a package'}                                | ${'/golang.org/x/tools@v0.1.1'}
  ${'module that is also a package'}                               | ${'/gocloud.dev@v0.22.0'}
  ${'really long import path'}                                     | ${'/github.com/envoyproxy/go-control-plane@v0.9.8/envoy/config/filter/network/http_connection_manager/v2'}
  ${'no documentation'}                                            | ${'/github.com/tendermint/tendermint@v0.34.10/cmd/contract_tests'}
  ${'package with multiple licenses'}                              | ${'/github.com/apache/thrift@v0.14.1?tab=licenses'}
  ${'package that exists in multiple modules at the same version'} | ${'/github.com/hashicorp/vault/api@v1.0.3'}
  ${'package not at latest version of module'}                     | ${'/golang.org/x/tools/cmd/vet'}
  ${'package with higher major version'}                           | ${'/rsc.io/quote'}
  ${'package with multi-GOOS'}                                     | ${'/github.com/creack/pty@v1.1.11'}
  ${'retracted package'}                                           | ${'/k8s.io/client-go@v1.5.2'}
  ${'deprecated package'}                                          | ${'/github.com/jba/bit'}
  ${'package with deprecated symbols'}                             | ${'/cuelang.org/go@v0.3.2/cue'}
  ${'package exists in other modules'}                             | ${'/rsc.io/quote?tab=versions'}
`;
