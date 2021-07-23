/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import './global-types';
import * as pg from './helpers/page';
import * as unit from './helpers/unit.page';
import { testcases } from './unit.testcases';

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
  await page.setViewport({ height: 1000, width: 500 });
});

afterAll(async () => {
  await page.close();
});

testcases('$name', async ({ path }) => {
  await page.goto(path);
  await prepare(page);
  const image = await page.screenshot();

  // eslint-disable-next-line jest/no-standalone-expect
  expect(image).toMatchImageSnapshot();
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});

async function prepare(page: Page): Promise<void> {
  await unit.prepare(page);
  await pg.$eval(page, '.Documentation-index', el => el.remove());
}
