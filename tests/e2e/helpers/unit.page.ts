/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import * as pg from './page';

/**
 * prepare gets the unit page ready for snapshot testing by changing the
 * imported by count to zero and hiding the header and footer to simplify
 * snapshot diffing.
 * @param page The page to prepare
 */
export async function prepare(page: Page): Promise<void> {
  await pg.prepare(page);
  await Promise.all([
    pg.$$eval(page, pg.select('UnitHeader-importedby', 'a'), els =>
      els.map(el => (el.innerHTML = 'Imported by: 0'))
    ),
    pg.$eval(page, '.go-Header', el => ((el as HTMLElement).style.visibility = 'hidden')),
    pg.$eval(page, '.go-Footer', el => ((el as HTMLElement).style.visibility = 'hidden')),
  ]);
  await page.evaluate(() => new Promise(r => setTimeout(r, 500)));
}

/**
 * snapshotId generates a snapshot identifier replacing characters that are not
 * allowed in filenames within go modules.
 * @param env 'desktop' or 'mobile'
 * @param path a pkg.go.dev url path
 * @returns a snapshot idenifier
 */
export function snapshotId(env: 'desktop' | 'mobile', path: string): string {
  return `unit-${env}-${path.replace(/[*<>?`'|/\\:]/g, '-')}`;
}
