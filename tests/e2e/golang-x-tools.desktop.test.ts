/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import * as pg from './helpers/page';
import * as golangxtools from './helpers/golang-x-tools.page';

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
  // Wait for header transition
  await new Promise(r => setTimeout(r, 250));
  const image = await page.screenshot();
  expect(image).toMatchImageSnapshot();
  await page.evaluate(() => window.scrollTo({ top: 0 }));
});

describe('readme', () => {
  test('expands', async () => {
    const expand = pg.select('readme-expand');
    await page.$eval(expand, el => el.scrollIntoView({ block: 'center' }));
    await page.click(expand);
    await scrollTop(page);
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
  });

  test('collapses', async () => {
    const collapse = pg.select('readme-collapse');
    await page.$eval(collapse, el => el.scrollIntoView({ block: 'center' }));
    await page.click(collapse);
    await scrollTop(page);
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });
});

describe('directories', () => {
  test('expand', async () => {
    const toggle = pg.select('directories-toggle');
    await page.$eval(toggle, el => el.scrollIntoView({ block: 'center' }));
    await page.click(toggle);
    await scrollTop(page);
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
  });

  test('collapse', async () => {
    const toggle = pg.select('directories-toggle');
    await page.$eval(toggle, el => el.scrollIntoView({ block: 'center' }));
    await page.click(toggle);
    await scrollTop(page);
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });
});

describe('jump to modal', () => {
  test('opens', async () => {
    await page.click(pg.select('jump-to-button'));
    await scrollTop(page);
    const expanded = await page.screenshot();
    expect(expanded).toMatchImageSnapshot();
  });

  test('closes', async () => {
    await page.click(pg.select('close-dialog'));
    await scrollTop(page);
    const collapsed = await page.screenshot();
    expect(collapsed).toMatchImageSnapshot();
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});

/**
 * scrollTop scrolls to the top of a given page and waits
 * a short amount of time for any style transitions to
 * complete. Used to make sure the documentation page
 * header has completed transitioning.
 * @param page the page to scroll.
 */
async function scrollTop(page: Page): Promise<void> {
  await page.evaluate(() => window.scrollTo({ top: 0 }));
  await new Promise(r => setTimeout(r, 250));
}
