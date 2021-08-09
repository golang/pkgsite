/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { configureToMatchImageSnapshot } from 'jest-image-snapshot';

let env = process.env.GO_DISCOVERY_E2E_ENVIRONMENT;
if (env != 'ci') {
  env = 'staging';
}

const snapshotDir = `tests/e2e/__image_snapshots__/${env}`;

// Extends jest to compare image snapshots.
const toMatchImageSnapshot = configureToMatchImageSnapshot({
  failureThreshold: 0.001,
  failureThresholdType: 'percent',
  customSnapshotsDir: snapshotDir,
  customSnapshotIdentifier: ({ defaultIdentifier, counter }) => {
    return defaultIdentifier.replace('test-ts', '').replace(`-${counter}`, '');
  },
});
expect.extend({ toMatchImageSnapshot });
