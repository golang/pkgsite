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

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
  await page.goto('/errors@go1.16.3');
  await unit.prepare(page);
});

afterAll(async () => {
  await page.close();
});

test.each`
  href
  ${'#pkg-overview'}
  ${'#pkg-index'}
  ${'#pkg-constants'}
  ${'#pkg-variables'}
  ${'#pkg-functions'}
  ${'#As'}
  ${'#pkg-types'}
  ${'#section-sourcefiles'}
`('doc outline $href', async ({ href }) => {
  await page.waitForSelector(`[href="#section-documentation"][aria-expanded="true"]`);
  await page.click(`[href="${href}"][role="treeitem"]`);
  const image = await page.screenshot();
  expect(image).toMatchImageSnapshot();
});

describe('jump to modal', () => {
  test('opens', async () => {
    await page.click(pg.select('jump-to-button'));
    const expanded = await page.screenshot();
    expect(expanded).toMatchImageSnapshot();
  });

  test('searches identifiers on input', async () => {
    await page.keyboard.type('Wrap');
    const inputWrap = await page.screenshot();
    expect(inputWrap).toMatchImageSnapshot();
  });

  test('jumps to selected identifier', async () => {
    await page.keyboard.press('Enter');
    const wrap = await page.screenshot();
    expect(wrap).toMatchImageSnapshot();
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
