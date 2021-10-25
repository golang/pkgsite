/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

let config = {
  testEnvironment: 'jsdom',
  preset: 'ts-jest',
  globals: {
    'ts-jest': {
      isolatedModules: true,
    },
  },
  moduleFileExtensions: ['ts', 'js'],
  testRunner: 'jest-circus/runner',
  testPathIgnorePatterns: ['tests/e2e'],
};

// When GO_DISCOVERY_E2E_ENVIRONMENT is not set to staging or prod, use the
// snapshots in tests/e2e/__snapshots__/ci. Otherwise, use
// tests/e2e/__snapshots__/staging. Data in staging and prod are always expected
// to be the same.
//
// eslint-disable-next-line no-undef
const env = process.env.GO_DISCOVERY_E2E_ENVIRONMENT;
let ignore = 'staging';
if (env === 'staging' || env === 'prod') {
  ignore = 'ci';
}

// eslint-disable-next-line no-undef
const e2e = process.argv.some(arg => arg.includes('e2e'));
if (e2e) {
  config = {
    ...config,
    setupFilesAfterEnv: ['<rootDir>/tests/e2e/setup.ts'],
    testEnvironment: '<rootDir>/tests/e2e/test-environment.js',
    snapshotResolver: '<rootDir>/tests/e2e/snapshotResolver.js',
    testTimeout: 60000,
    testPathIgnorePatterns: ['static', 'tests/e2e/__snapshots__/' + ignore],
  };
}

// eslint-disable-next-line no-undef
module.exports = config;
