/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import puppeteer, { Page } from 'puppeteer';

describe('Unit Details - golang.org/x/pkgsite - desktop', () => {
  let page: Page;
  beforeAll(async () => {
    page = await newPage();
    await page.goto(baseURL + '/golang.org/x/pkgsite');
    await prepare(page);
  });

  afterAll(async () => {
    await page.close();
  });

  test('viewport matches image snapshot', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot({});
  });

  test('fixed header appears after scrolling', async () => {
    await page.mouse.wheel({ deltaY: 250 });
    // wait for css transition
    await page.waitForTimeout(500);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });
});

describe('Unit Details - golang.org/x/pkgsite - mobile', () => {
  let page: Page;
  beforeAll(async () => {
    page = await newPage();
    await page.emulate(puppeteer.devices['Pixel 2']);
    await page.goto(baseURL + '/golang.org/x/pkgsite');
    await prepare(page);
  });

  afterAll(async () => {
    await page.close();
  });

  test('viewport matches image snapshot', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test('fixed header appears after scrolling', async () => {
    await page.mouse.wheel({ deltaY: 250 });
    // wait for css transition
    await page.waitForTimeout(500);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});

/**
 * prepare gets the page ready for snapshot testing by rewriting highly
 * variable page content to constant values.
 * @param page The page to prepare
 */
async function prepare(page: Page): Promise<void> {
  await Promise.all([
    page.$eval(
      '[data-test-id="UnitHeader-version"] a',
      el => ((el as HTMLElement).innerHTML = '<span>Version: </span>v0.0.0')
    ),
    page.$eval(
      '[data-test-id="UnitHeader-commitTime"]',
      el => ((el as HTMLElement).innerHTML = 'Published: Apr 16, 2021')
    ),
    page.$$eval('[data-test-id="UnitHeader-imports"] a', els =>
      els.map(el => (el.innerHTML = 'Imports: 0'))
    ),
    page.$$eval('[data-test-id="UnitHeader-importedby"] a', els =>
      els.map(el => (el.innerHTML = 'Imported by: 0'))
    ),
  ]);
}
