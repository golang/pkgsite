/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import './global-types';
import * as pg from './helpers/page';
import * as search from './helpers/search.page';

let page: Page;

beforeEach(async () => {
  page = await pg.newPage();
  await page.goto('/search?q=http');
  await search.prepare(page);
});

afterEach(async () => {
  await page.close();
});

test('desktop accessibility tree', async () => {
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
});

test('desktop screenshot', async () => {
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
});

test('mobile accessibility tree', async () => {
  await page.setViewport({ width: 411, height: 731 });
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
});

test('mobile screenshot', async () => {
  await page.setViewport({ width: 411, height: 731 });
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
