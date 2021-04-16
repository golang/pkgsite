/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import puppeteer, { Page } from 'puppeteer';

describe('Homepage', () => {
  let page: Page;

  beforeEach(async () => {
    page = await newPage();
    await page.goto(baseURL);
  });

  afterEach(async () => {
    await page.close();
  });

  test('accessibility tree matches snapshot', async () => {
    const a11yTree = await page.accessibility.snapshot();
    expect(a11yTree).toMatchSnapshot();
  });

  test('desktop viewport matches image snapshot', async () => {
    await page.$eval('[data-test-id="homepage-search"]', e => (e as HTMLInputElement).blur());
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test('mobile viewport matches image snapshot', async () => {
    await page.emulate(puppeteer.devices['Pixel 2']);
    await page.$eval('[data-test-id="homepage-search"]', e => (e as HTMLInputElement).blur());
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });
});
