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

/**
 * tab represents tabs of the unit page. The string value is added
 * to page URLs as part of the tab query param (e.g. ?tab=licenses).
 */
export enum tab {
  LICENSES = 'licenses',
  IMPORTS = 'imports',
  VERSIONS = 'versions',
}

/**
 * id represents the standard identifier values that appear on the
 * unit page. The string value is added to page URLs as part of the
 * URL fragment (e.g., pkg.go.dev/std#section-directories).
 */
enum id {
  README = '#section-readme',
  DOCUMENTATION = '#section-documentation',
  OVERVIEW = '#pkg-overview',
  INDEX = '#pkg-index',
  EXAMPLES = '#pkg-examples',
  CONSTANTS = '#pkg-constants',
  VARIABLES = '#pkg-variables',
  FUNCTIONS = '#pkg-functions',
  TYPES = '#pkg-types',
  SOURCEFILES = '#section-sourcefiles',
  DIRECTORIES = '#section-directories',
}

/**
 * TestCase represents a snapshot test case for a unit page.
 */
interface TestCase {
  /**
   * name is the name of the test case that provides context for why
   * it is useful.
   */
  name: string;
  /**
   * path is the unit's path.
   */
  path: string;
  /**
   * tabs are additional tabs beyond the main page to snapshot.
   */
  tabs: tab[];
  /**
   * ids are additional areas of the snapshot beyond the top of
   * the page to snapshot.
   */
  ids: string[];
}

export const testcases: TestCase[] = [
  {
    name: 'standard library package',
    path: '/errors@go1.16.3',
    tabs: [tab.LICENSES, tab.IMPORTS],
    ids: [
      id.DOCUMENTATION,
      id.OVERVIEW,
      id.INDEX,
      id.EXAMPLES,
      id.CONSTANTS,
      id.VARIABLES,
      id.FUNCTIONS,
      id.TYPES,
      id.SOURCEFILES,
    ],
  },
  {
    name: 'really long import path',
    path:
      '/github.com/envoyproxy/go-control-plane@v0.9.8/envoy/config/filter/network/http_connection_manager/v2',
    tabs: [],
    ids: [],
  },
  {
    name: 'package that exists in multiple modules at the same versions',
    path: '/github.com/hashicorp/vault/api@v1.0.3',
    tabs: [],
    ids: [
      id.DOCUMENTATION,
      id.INDEX,
      id.CONSTANTS,
      id.VARIABLES,
      id.FUNCTIONS,
      id.TYPES,
      id.SOURCEFILES,
    ],
  },
  {
    name: 'no documentation',
    path: '/github.com/tendermint/tendermint@v0.34.10/cmd/contract_tests',
    tabs: [],
    ids: [id.DOCUMENTATION, id.SOURCEFILES],
  },
  {
    name: 'module that is also a package',
    path: '/gocloud.dev@v0.22.0',
    tabs: [],
    ids: [id.DOCUMENTATION, id.OVERVIEW, id.SOURCEFILES, id.DIRECTORIES],
  },
  {
    name: 'package not at latest version of a module',
    path: '/golang.org/x/tools/cmd/vet',
    tabs: [],
    ids: [id.DOCUMENTATION, id.OVERVIEW, id.SOURCEFILES, id.DIRECTORIES],
  },
  {
    name: 'module that is not a package',
    path: '/golang.org/x/tools@v0.1.1',
    tabs: [tab.LICENSES],
    ids: [id.README, id.DIRECTORIES],
  },
  {
    name: 'standard library',
    path: '/std@go1.16.3',
    tabs: [tab.LICENSES, tab.IMPORTS],
    ids: [id.DIRECTORIES],
  },
  {
    name: 'package with multiple licenses',
    path: '/github.com/apache/thrift@v0.14.1',
    tabs: [tab.LICENSES],
    ids: [id.README, id.DIRECTORIES],
  },
  {
    name: 'package with higher major version',
    path: '/rsc.io/quote',
    tabs: [tab.VERSIONS],
    ids: [
      id.DOCUMENTATION,
      id.OVERVIEW,
      id.INDEX,
      id.CONSTANTS,
      id.VARIABLES,
      id.FUNCTIONS,
      id.TYPES,
      id.SOURCEFILES,
    ],
  },
  {
    name: 'package with multi-GOOS',
    path: '/github.com/creack/pty@v1.1.11',
    tabs: [],
    ids: [id.README, id.DOCUMENTATION, id.SOURCEFILES, id.DIRECTORIES],
  },
  {
    name: 'retracted package',
    path: '/k8s.io/client-go@v1.5.2',
    tabs: [],
    ids: [id.README, id.DOCUMENTATION, id.SOURCEFILES, id.DIRECTORIES],
  },
  {
    name: 'deprecated package',
    path: '/github.com/jba/bit',
    tabs: [],
    ids: [id.README, id.DOCUMENTATION, id.SOURCEFILES, id.DIRECTORIES],
  },
  {
    name: 'package with deprecated symbols',
    path: '/cuelang.org/go@v0.3.2/cue',
    tabs: [],
    ids: [id.INDEX, '#Merge'],
  },
];
