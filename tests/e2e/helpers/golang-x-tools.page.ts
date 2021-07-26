import { Page } from 'puppeteer';

import * as pg from './page';
import * as unit from './unit.page';

/**
 * prepare gets the /golang.org/x/tools@v0.1.1 frontend page ready for snapshot
 * tests by rewriting highly variable page content to constant values.
 * @param page The page to prepare
 */
export async function prepare(page: Page): Promise<void> {
  await unit.prepare(page);
  await pg.$$eval(page, pg.select('UnitHeader-imports', 'a'), els =>
    els.map(el => (el.innerHTML = 'Imports: 0'))
  );
}
