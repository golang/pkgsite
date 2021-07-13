/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

let config = {
  preset: 'ts-jest',
  globals: {
    'ts-jest': {
      isolatedModules: true,
    },
  },
  moduleFileExtensions: ['ts', 'js'],
  testRunner: 'jest-circus/runner',
};

// eslint-disable-next-line no-undef
const e2e = process.argv.some(arg => arg.includes('e2e'));
if (e2e) {
  config = {
    ...config,
    setupFilesAfterEnv: ['<rootDir>/tests/e2e/setup.ts'],
    testEnvironment: '<rootDir>/tests/e2e/test-environment.js',
    testTimeout: 60000,
  };
}

// eslint-disable-next-line no-undef
module.exports = config;
