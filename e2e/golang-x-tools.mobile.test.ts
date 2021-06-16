/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import puppeteer, { Page } from 'puppeteer';

import './global-types';
import * as pg from './helpers/page';
import * as golangxtools from './helpers/golang-x-tools.page';

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
  await page.emulate(puppeteer.devices['Pixel 2']);
  await page.goto('/golang.org/x/tools@v0.1.1');
  await golangxtools.prepare(page);
});

afterAll(async () => {
  await page.close();
});

test('fixed header appears after scrolling', async () => {
  await page.evaluate(() => window.scrollTo({ top: 250 }));
  const image = await page.screenshot();
  expect(image).toMatchImageSnapshot();
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
