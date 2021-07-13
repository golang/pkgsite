/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import './global-types';
import * as pg from './helpers/page';
import * as golangxtools from './helpers/golang-x-tools.page.ts';

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
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
  await page.evaluate(() => window.scrollTo({ top: 0 }));
});

describe('readme', () => {
  test('expands', async () => {
    await page.click(pg.select('readme-expand'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
  });

  test('collapses', async () => {
    await page.click(pg.select('readme-collapse'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });
});

describe('directories', () => {
  test('expand', async () => {
    await page.click(pg.select('directories-toggle'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
  });

  test('collapse', async () => {
    await page.click(pg.select('directories-toggle'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });
});

describe('jump to modal', () => {
  test('opens', async () => {
    await page.click(pg.select('jump-to-button'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot();
    expect(expanded).toMatchImageSnapshot();
  });

  test('closes', async () => {
    await page.click(pg.select('close-dialog'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot();
    expect(collapsed).toMatchImageSnapshot();
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
