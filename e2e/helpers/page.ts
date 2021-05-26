/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { DirectNavigationOptions, Page } from 'puppeteer';

import '../global-types';

const {
  GO_DISCOVERY_E2E_AUTHORIZATION = null,
  GO_DISCOVERY_E2E_BASE_URL = 'http://host.docker.internal:8080',
  GO_DISCOVERY_E2E_QUOTA_BYPASS = null,
} = process.env;

/**
 * newPage opens a new chrome tab, sets up a request intercept
 * to provide an authorization header for relevant requests, and
 * prefixes page.goto urls with the base URL for the test current
 * environment.
 * @returns a new page context
 */
export async function newPage(): Promise<Page> {
  const page = await browser.newPage();
  if (GO_DISCOVERY_E2E_AUTHORIZATION) {
    await page.setRequestInterception(true);
    page.on('request', r => {
      const url = new URL(r.url());
      let headers = r.headers();
      if (url.origin === GO_DISCOVERY_E2E_BASE_URL) {
        headers = {
          ...r.headers(),
          Authorization: `Bearer ${GO_DISCOVERY_E2E_AUTHORIZATION}`,
          'X-Go-Discovery-Auth-Bypass-Quota': GO_DISCOVERY_E2E_QUOTA_BYPASS,
        };
      }
      r.continue({ headers });
    });
  }
  page.on('pageerror', err => {
    this.global.pageErrors.push(err);
  });
  const go = page.goto;
  page.goto = (path: string, opts?: DirectNavigationOptions) =>
    go.call(page, GO_DISCOVERY_E2E_BASE_URL + path, opts);
  return page;
}

/**
 * select will create a data-test-id attribute selector for a given test id.
 * @param testId the test id of the element to select.
 * @param rest a place to add combinators and additional selectors.
 * @returns an attribute selector.
 */
export function select(testId: string, rest = ''): string {
  return `[data-test-id="${testId}"] ${rest}`;
}

/**
 * prepare disables page transitions and animations.
 * @param page The page to prepare
 */
export async function prepare(page: Page): Promise<void> {
  await Promise.all([
    page.addStyleTag({
      content: `
         *,
         *::after,
         *::before {
             transition-delay: 0s !important;
             transition-duration: 0s !important;
             animation-delay: -0.0001s !important;
             animation-duration: 0s !important;
             animation-play-state: paused !important;
             caret-color: transparent;
         }`,
    }),
  ]);
}

/**
 * $eval wraps page.$eval to check if an element exists before
 * attempting to run the callback.
 * @param page the current page
 * @param selector a css selector
 * @param cb an operation to perform on the selected element
 */
export async function $eval(
  page: Page,
  selector: string,
  cb?: (el: Element) => unknown
): Promise<void> {
  if (await page.$(selector)) {
    await page.$eval(selector, cb);
  }
}

/**
 * $$eval wraps page.$$eval to check if an element exists before
 * attempting to run the callback.
 * @param page the current page
 * @param selector a css selector
 * @param cb an operation to perform on an array of the selected elements
 */
export async function $$eval(
  page: Page,
  selector: string,
  cb?: (els: Element[]) => unknown
): Promise<void> {
  if (await page.$(selector)) {
    await page.$$eval(selector, cb);
  }
}

/**
 * TestOptions are options for basic page tests.
 */
export interface TestOptions {
  /**
   * path is the pathname of the page to visit.
   */
  path: string;

  /**
   * mobile will set the page to a mobile viewport size.
   */
  mobile?: boolean;

  /**
   * prepare will prepare the page for screenshot.
   */
  prepare?: typeof prepare;
}

/**
 * a11ySnapshotTest asserts that the a11y tree matches the snapshot.
 * @param page a page object.
 * @param opts test options.
 */
export async function a11ySnapshotTest(
  page: Page,
  opts: TestOptions = { path: '' }
): Promise<void> {
  if (opts.mobile) {
    await page.setViewport({ width: 411, height: 731 });
  }
  await page.goto(opts.path);
  await (opts.prepare ?? prepare)(page);
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
  await page.close();
}

/**
 * fullScreenshotTest asserts that the full page screenshot matches the
 * image snapshot.
 * @param path a page object.
 * @param opts test options.
 */
export async function fullScreenshotTest(
  page: Page,
  opts: TestOptions = { path: '' }
): Promise<void> {
  if (opts.mobile) {
    await page.setViewport({ width: 411, height: 731 });
  }
  await page.goto(opts.path);
  await (opts.prepare ?? prepare)(page);
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
  await page.close();
}
