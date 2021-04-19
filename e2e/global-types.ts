/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Browser, Page } from 'puppeteer';

/**
 * global declares global variables available in e2e test files.
 */
declare global {
  /**
   * A controller for the instance of Chrome used in for the tests.
   */
  const browser: Browser;

  /**
   * The baseURL for pkgsite pages (e.g., https://staging-pkg.go.dev).
   */
  const baseURL: string;

  /**
   * newPage resolves to a new Page object. The Page object provides methods to
   * interact with a single Chrome tab.
   */
  const newPage: () => Promise<Page>;

  /**
   * pageErrors is a record of uncaught exceptions that have occured on a page.
   */
  const pageErrors: Error[];
}
