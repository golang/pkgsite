/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { Page } from 'puppeteer';

import * as pg from './page';

/**
 * prepare gets the search page ready for snapshot tests by rewriting highly
 * variable page content to constant values.
 * @param page The page to prepare
 */
export async function prepare(page: Page): Promise<void> {
  await pg.prepare(page);
  await Promise.all([
    pg.$$eval(page, '[data-test-id="snippet-title"]', els =>
      els.map(el => {
        el.innerHTML = 'net/http/pprof';
        (el as HTMLAnchorElement).href = 'net/http/pprof';
      })
    ),
    pg.$$eval(page, '[data-test-id="snippet-synopsis"]', els =>
      els.map(el => {
        el.innerHTML =
          'Package pprof serves via its HTTP server runtime profiling ' +
          'data in the format expected by the pprof visualization tool.';
      })
    ),
    pg.$$eval(page, '[data-test-id="snippet-version"]', els =>
      els.map(el => (el.innerHTML = 'go1.16.3'))
    ),
    pg.$$eval(page, '[data-test-id="snippet-published"]', els =>
      els.map(el => (el.innerHTML = 'Apr 1, 2021'))
    ),
    pg.$$eval(page, '[data-test-id="snippet-importedby"]', els =>
      els.map(el => (el.innerHTML = '11632'))
    ),
    pg.$$eval(page, '[data-test-id="snippet-license"]', els =>
      els.map(el => (el.innerHTML = 'BSD-3-Clause'))
    ),
    pg.$$eval(page, '[data-test-id="results-total"]', els =>
      els.map(el => (el.innerHTML = '1 – 25 of 125 results'))
    ),
  ]);
}
