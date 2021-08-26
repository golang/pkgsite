/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { configureToMatchImageSnapshot } from 'jest-image-snapshot';

// When GO_DISCOVERY_E2E_ENVIRONMENT is not set to staging or prod, use the
// snapshots in tests/e2e/__snapshots__/ci. Otherwise, use
// tests/e2e/__snapshots__/staging. Data in taging and prod are always expected
// to be the same.
//
// eslint-disable-next-line no-undef
let env = process.env.GO_DISCOVERY_E2E_ENVIRONMENT;
if (env === 'staging' || env === 'prod') {
  env = 'staging';
} else {
  env = 'ci';
}
const snapshotDir = `tests/e2e/__image_snapshots__/${env}`;

// Extends jest to compare image snapshots.
const toMatchImageSnapshot = configureToMatchImageSnapshot({
  failureThreshold: 0.001,
  failureThresholdType: 'percent',
  customSnapshotsDir: snapshotDir,
  customDiffConfig: {
    diffColorAlt: [0, 255, 0],
  },
  customSnapshotIdentifier: ({ defaultIdentifier, counter }) => {
    return defaultIdentifier.replace('test-ts', '').replace(`-${counter}`, '');
  },
});
expect.extend({ toMatchImageSnapshot });
