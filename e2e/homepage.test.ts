/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './globals';
import puppeteer from 'puppeteer';

const baseUrl = process.env.FRONTEND_URL ?? '';

describe('Homepage', () => {
  beforeEach(async () => {
    await page.goto(baseUrl);
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
});
