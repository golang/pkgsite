/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import type { Page } from 'puppeteer';

interface TestArgs {
  name: string;
  path: string;
}

interface TestCases {
  (name: string, fn: (arg: TestArgs) => unknown, timeout?: number): unknown;
}

export const testcases: TestCases = test.each`
  name                       | path                                    | prepare
  ${'badge'}                 | ${'/badge'}                             | ${null}
  ${'error'}                 | ${'/bad.package@v1.0-badversion'}       | ${null}
  ${'404 with fetch button'} | ${'/github.com/package/does/not/exist'} | ${null}
  ${'home'}                  | ${'/'}                                  | ${prepareHome}
  ${'license policy'}        | ${'/license-policy'}                    | ${null}
  ${'search help'}           | ${'/search-help'}                       | ${null}
  ${'sub-repositories'}      | ${'/golang.org/x'}                      | ${null}
`;

/**
 * prepareHome selects the first element in the homepage search tips carousel.
 * @param page homepage
 */
async function prepareHome(page: Page) {
  const dot = await page.$('.go-Carousel-dot');
  await dot.click();
}
