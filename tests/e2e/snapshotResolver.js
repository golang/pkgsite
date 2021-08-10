/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// When GO_DISCOVERY_E2E_ENVIRONMENT is not set to staging or prod, use the
// snapshots in tests/e2e/__snapshots__/ci. Otherwise, use
// tests/e2e/__snapshots__/staging. Data in staging and prod are always
// expected to be the same.
//
// eslint-disable-next-line no-undef
let env = process.env.GO_DISCOVERY_E2E_ENVIRONMENT;
if (env === 'staging' || env === 'prod') {
  env = 'staging';
} else {
  env = 'ci';
}
const snapshotDir = `tests/e2e/__snapshots__/${env}`;

// eslint-disable-next-line no-undef
module.exports = {
  // resolves from test to snapshot path
  resolveSnapshotPath: (testPath, snapshotExtension) =>
    testPath.replace('tests/e2e', snapshotDir) + snapshotExtension,

  // resolves from snapshot to test path
  resolveTestPath: (snapshotFilePath, snapshotExtension) =>
    snapshotFilePath.replace(snapshotDir, 'tests/e2e').slice(0, -snapshotExtension.length),

  // Example test path, used for preflight consistency check of the implementation above
  testPathForConsistencyCheck: 'tests/e2e/example.test.js',
};
