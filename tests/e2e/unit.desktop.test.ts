/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import * as pg from './helpers/page';
import * as unit from './helpers/unit.page';
import { testcases, tab } from './unit.testcases';

const { CI = false } = process.env;

let page: Page;

beforeAll(async () => {
  page = await pg.newPage();
});

afterAll(async () => {
  await page.close();
});

for (const tc of testcases) {
  // Snapshot top of unit page.
  test(`main - ${tc.name} (${tc.path})`, async () => {
    await page.goto(`${tc.path}`);
    await unit.prepare(page);
    const image = await page.screenshot();
    expect(image).toMatchImageSnapshot({ customSnapshotIdentifier: snapshotId(tc.path) });
  });

  // Snapshot additional unit page sections.
  for (const id of tc.ids) {
    const path = `${tc.path}${id}`;
    test(`main - ${tc.name} (${path})`, async () => {
      await page.goto(path);
      await unit.prepare(page);
      const image = await page.screenshot();
      expect(image).toMatchImageSnapshot({ customSnapshotIdentifier: snapshotId(path) });
    });
  }

  // Snapshot additional unit page tabs.
  for (const t of tc.tabs) {
    // Skip versions tab in CI.
    if (CI && t == tab.VERSIONS) continue;
    const path = `${tc.path}?tab=${t}`;
    test(`${t} - ${tc.name} (${path})`, async () => {
      await page.goto(path);
      await unit.prepare(page);
      const image = await page.screenshot();
      expect(image).toMatchImageSnapshot({ customSnapshotIdentifier: snapshotId(path) });
    });
  }
}

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});

function snapshotId(path: string): string {
  return 'unit-desktop-' + path.replace(/\//g, '-');
}
