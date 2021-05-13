/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import './global-types';
import { testcases } from './basic.testcases';
import * as pg from './helpers/page';

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
  await page.setViewport({ width: 411, height: 731 });
});

afterAll(async () => {
  await page.close();
});

testcases('$name accessibility tree', async ({ path, prepare }) => {
  await page.goto(path);
  if (prepare) await prepare(page);
  const a11yTree = await page.accessibility.snapshot();
  expect(a11yTree).toMatchSnapshot();
});

testcases('$name screenshot', async ({ path, prepare }) => {
  await page.goto(path);
  if (prepare) await prepare(page);
  const image = await page.screenshot({ fullPage: true });
  expect(image).toMatchImageSnapshot();
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
