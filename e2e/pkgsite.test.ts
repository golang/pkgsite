/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './globals';
import puppeteer, { Page } from 'puppeteer';

const baseUrl = process.env.FRONTEND_URL ?? '';

describe('golang.org/x/pkgsite', () => {
  let page: Page;

  beforeAll(async () => {
    page = await browser.newPage();
    await page.goto(baseUrl);
    await page.evaluate(() =>
      fetch(`/fetch/golang.org/x/pkgsite@v0.0.0-20210216165259-5867665b19ca`, { method: 'POST' })
    );
  }, 30000);

  beforeEach(async () => {
    page = await browser.newPage();
    await page.goto(baseUrl + '/golang.org/x/pkgsite');
  });

  afterEach(async () => {
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

  test('desktop fixed header appears after scrolling', async () => {
    await page.mouse.wheel({ deltaY: 250 });
    // wait for css transition
    await page.waitForTimeout(500);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });

  test('mobile fixed header appears after scrolling', async () => {
    await page.emulate(puppeteer.devices['Pixel 2']);
    await page.mouse.wheel({ deltaY: 250 });
    // wait for css transition
    await page.waitForTimeout(500);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });
});
