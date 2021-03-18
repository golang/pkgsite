/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './globals';
import puppeteer, { Page } from 'puppeteer';

const baseUrl = process.env.FRONTEND_URL ?? '';

const selectors = {
  fetchButton: '.js-fetchButton',
  fetchMessage: '.js-fetchMessage',
};

describe('Frontend Fetch', () => {
  let page: Page;

  beforeAll(async () => {
    page = await browser.newPage();
    await page.goto(baseUrl + '/golang.org/x/tools/gopls@v0.6.6');
  });

  afterAll(async () => {
    await page.close();
  });

  test('accessibility tree matches snapshot', async () => {
    const a11yTree = await page.accessibility.snapshot();
    expect(a11yTree).toMatchSnapshot();
  });

  test('desktop viewport matches image snapshot', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test('mobile viewport matches image snapshot', async () => {
    await page.emulate(puppeteer.devices['Pixel 2']);
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test('clicking fetch button fetches module and navigates to page', async () => {
    expect(await page.title()).toBe('404 Not Found · pkg.go.dev');

    await page.click(selectors.fetchButton);
    const message = await page.$eval(selectors.fetchMessage, el => el.textContent);
    expect(message).toBe('Fetching golang.org/x/tools/gopls@v0.6.6');

    await page.waitForNavigation();
    expect(await page.title()).toBe('gopls · pkg.go.dev');
  }, 30000);
});
