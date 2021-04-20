/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import puppeteer, { Page } from 'puppeteer';

describe('pkgsite (desktop)', () => {
  let page: Page;
  beforeAll(async () => {
    page = await newPage();
    await page.goto(baseURL + '/golang.org/x/pkgsite');
    await prepare(page);
  });

  afterAll(async () => {
    await page.close();
  });

  test('accessibility tree', async () => {
    const a11yTree = await page.accessibility.snapshot();
    expect(a11yTree).toMatchSnapshot();
  });

  test('full page', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot({});
  });

  test('fixed header appears after scrolling', async () => {
    await page.evaluate(() => window.scrollTo({ top: 250 }));
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });

  test('readme expand and collapse', async () => {
    await page.click(select('readme-expand'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
    await page.click(select('readme-collapse'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });

  test('directories expand and collapse', async () => {
    await page.click(select('directories-toggle'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot({ fullPage: true });
    expect(expanded).toMatchImageSnapshot();
    await page.click(select('directories-toggle'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot({ fullPage: true });
    expect(collapsed).toMatchImageSnapshot();
  });

  test('jump to without identifiers', async () => {
    await page.click(select('jump-to-button'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const expanded = await page.screenshot();
    expect(expanded).toMatchImageSnapshot();
    await page.click(select('close-dialog'));
    await page.evaluate(() => window.scrollTo({ top: 0 }));
    const collapsed = await page.screenshot();
    expect(collapsed).toMatchImageSnapshot();
  });
});

describe('pkgsite (mobile)', () => {
  let page: Page;
  beforeAll(async () => {
    page = await newPage();
    await page.emulate(puppeteer.devices['Pixel 2']);
    await page.goto(baseURL + '/golang.org/x/pkgsite');
    await prepare(page);
  });

  afterAll(async () => {
    await page.close();
  });

  test('accessibility tree', async () => {
    const a11yTree = await page.accessibility.snapshot();
    expect(a11yTree).toMatchSnapshot();
  });

  test('full page', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test('fixed header appears after scrolling', async () => {
    await page.evaluate(() => window.scrollTo({ top: 250 }));
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });
});

describe('derrors', () => {
  let page: Page;
  beforeAll(async () => {
    page = await newPage();
    await page.goto(baseURL + '/golang.org/x/pkgsite/internal/derrors');
    await prepare(page);
  });

  afterAll(async () => {
    await page.close();
  });

  test('accessibility tree', async () => {
    const a11yTree = await page.accessibility.snapshot();
    expect(a11yTree).toMatchSnapshot();
  });

  test('full page', async () => {
    const image = await page.screenshot({ fullPage: true });
    expect(image).toMatchImageSnapshot();
  });

  test.each`
    href
    ${'#section-documentation'}
    ${'#pkg-overview'}
    ${'#pkg-index'}
    ${'#pkg-constants'}
    ${'#pkg-variables'}
    ${'#pkg-functions'}
    ${'#Add'}
    ${'#pkg-types'}
    ${'#StackError'}
    ${'#NewStackError'}
    ${'#section-sourcefiles'}
  `('doc outline $href', async ({ href }) => {
    await page.click(`[href="${href}"][role="treeitem"]`);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot();
  });

  test('jump to with identifiers', async () => {
    await page.click(select('jump-to-button'));
    const expanded = await page.screenshot();
    expect(expanded).toMatchImageSnapshot();
    await page.keyboard.type('Wrap');
    const inputWrap = await page.screenshot();
    expect(inputWrap).toMatchImageSnapshot();
    await page.keyboard.press('Enter');
    const wrap = await page.screenshot();
    expect(wrap).toMatchImageSnapshot();
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});

/**
 * select will create a data-test-id attribute selector for a given test id.
 * @param testId the test id of the element to select.
 * @param rest a place to add combinators and additional selectors.
 * @returns an attribute selector.
 */
function select(testId: string, rest = ''): string {
  return `[data-test-id="${testId}"] ${rest}`;
}

/**
 * prepare gets the page ready for snapshot testing by rewriting highly
 * variable page content to constant values.
 * @param page The page to prepare
 */
async function prepare(page: Page): Promise<void> {
  await Promise.all([
    // Add styles to disable animation transitions.
    page.addStyleTag({
      content: `
        *,
        *::after,
        *::before {
            transition-delay: 0s !important;
            transition-duration: 0s !important;
            animation-delay: -0.0001s !important;
            animation-duration: 0s !important;
            animation-play-state: paused !important;
        }`,
    }),
    page.$eval(
      select('UnitHeader-version', 'a'),
      el => ((el as HTMLElement).innerHTML = '<span>Version: </span>v0.0.0')
    ),
    page.$eval(
      select('UnitHeader-commitTime'),
      el => ((el as HTMLElement).innerHTML = 'Published: Apr 16, 2021')
    ),
    page.$$eval(select('UnitHeader-imports', 'a'), els =>
      els.map(el => (el.innerHTML = 'Imports: 0'))
    ),
    page.$$eval(select('UnitHeader-importedby', 'a'), els =>
      els.map(el => (el.innerHTML = 'Imported by: 0'))
    ),
  ]);
}
