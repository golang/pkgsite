/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import puppeteer, { Page } from 'puppeteer';

let page: Page;

beforeAll(async () => {
  page = await newPage();
  await page.goto(baseURL + '/search-help');
});

afterAll(async () => {
  await page.close();
});

test('accessibility tree (desktop)', async () => {
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
});

test('full page (desktop)', async () => {
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
});

test('accessibility tree (mobile)', async () => {
  await page.emulate(puppeteer.devices['Pixel 2']);
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
});

test('full page (mobile)', async () => {
  await page.emulate(puppeteer.devices['Pixel 2']);
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
