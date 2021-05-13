/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Browser } from 'puppeteer';

/**
 * global declares global variables available in e2e test files.
 */
declare global {
  /**
   * A controller for the instance of Chrome used in for the tests.
   */
  const browser: Browser;

  /**
   * pageErrors is a record of uncaught exceptions that have occured in a test suite.
   */
  const pageErrors: Error[];
}
