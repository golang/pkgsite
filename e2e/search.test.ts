/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import * as pg from './helpers/page';
import * as search from './helpers/search.page';

test('desktop accessibility tree', async () => {
  const page = await pg.newPage();
  await pg.a11ySnapshotTest(page, {
    path: '/search?q=http',
    prepare: search.prepare,
  });
});

test('desktop screenshot', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?q=http',
    prepare: search.prepare,
  });
});

test('mobile accessibility tree', async () => {
  const page = await pg.newPage();
  await pg.a11ySnapshotTest(page, {
    path: '/search?q=http',
    mobile: true,
    prepare: search.prepare,
  });
});

test('mobile screenshot', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?q=http',
    mobile: true,
    prepare: search.prepare,
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
